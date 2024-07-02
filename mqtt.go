package main

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"log"
	"strings"
	"time"

	mqttc "github.com/eclipse/paho.mqtt.golang"
	"github.com/irsl/broker-ari/arimsgs"
	mqtts "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
)

const (
	MQTT_UPSTREAM_BROKER_ADDR = "ssl://broker-ari.everyware-cloud.com:8883"
)

var (
	clientMap = map[string]mqttClient{}
	server    *mqtts.Server
)

type mqttClient struct {
	client       mqttc.Client
	birth        map[string]string
	params       map[string]int32
	paramsLimits map[string]arimsgs.ParameterLimit
	cWh          *arimsgs.ConsumptionMsg
	errors       *arimsgs.ParametersMsg
}

type AuthHook struct {
	auth.Hook
	server *mqtts.Server
}

func mqtt_log_Printf(t string, params ...any) {
	if !Config.Mqtt_debug {
		return
	}
	log.Printf(t, params...)
}

func (h *AuthHook) OnPacketRead(cl *mqtts.Client, pk packets.Packet) (packets.Packet, error) {
	// mqtt_log_Printf("OnPacketRead:  %+v", pk)
	if pk.Connect.WillFlag && len(pk.Connect.WillPayload) == 0 {
		// they violate [MQTT-3.1.2-9]
		mqtt_log_Printf("Patching WILL payload")
		pk.Connect.WillPayload = []byte{0}
	}
	return pk, nil
}
func (h *AuthHook) OnACLCheck(cl *mqtts.Client, topic string, write bool) bool {
	mqtt_log_Printf("OnACLCheck: topic: %v, write: %v", topic, write)
	return true
}
func (h *AuthHook) OnConnectAuthenticate(cl *mqtts.Client, pk packets.Packet) bool {
	// mqtt_log_Printf("OnConnectAuthenticate: %v connect params: %+v", cl.ID, pk.Connect)
	mqtt_log_Printf("OnConnectAuthenticate: %v, username: %v, password: %+v", cl.ID, string(pk.Connect.Username), string(pk.Connect.Password))

	if Config.Mqtt_proxy_upstream == "" {
		// proxying is disabled
		return true
	}

	opts := mqttc.NewClientOptions()
	opts.AddBroker(Config.Mqtt_proxy_upstream)
	opts.SetKeepAlive(0xeb)
	opts.SetBinaryWill(pk.Connect.WillTopic, []byte{}, pk.Connect.WillQos, pk.Connect.WillRetain)
	opts.SetAutoReconnect(true)
	opts.SetClientID(cl.ID)
	opts.SetUsername(string(pk.Connect.Username))
	opts.SetPassword(string(pk.Connect.Password))
	opts.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	opts.SetDefaultPublishHandler(func(client mqttc.Client, msg mqttc.Message) {
		mqtt_log_Printf("OnPublish from the upstream: %s, topic: %s payload: %s", cl.ID, msg.Topic(),
			base64.StdEncoding.EncodeToString(msg.Payload()))
		h.server.Publish(msg.Topic(), msg.Payload(), msg.Retained(), msg.Qos())
	})
	mqtt_log_Printf("connecting to the upstream mqtt broker: %v, %v", cl.ID, Config.Mqtt_proxy_upstream)
	client := mqttc.NewClient(opts)
	if token := client.Connect(); token.WaitTimeout(5*time.Second) && token.Error() != nil {
		mqtt_log_Printf("Unable to connect to the upstream mqtt broker: %v", token.Error())
		return false
	}

	clientMap[cl.ID] = mqttClient{
		client: client,
	}

	return true
}
func (h *AuthHook) OnDisconnect(cl *mqtts.Client, err error, expire bool) {
	mqtt_log_Printf("OnDisconnect on the local broker: %v: %v", cl.ID, err)
	h.server.Clients.Delete(cl.ID)
	c := clientMap[cl.ID]
	if c.client != nil {
		c.client.Disconnect(0)
	}
	delete(clientMap, cl.ID)
}
func (h *AuthHook) Provides(b byte) bool {
	return bytes.Contains([]byte{mqtts.OnConnectAuthenticate, mqtts.OnACLCheck, mqtts.OnPacketRead, mqtts.OnDisconnect}, []byte{b})
}

type MsgHook struct {
	mqtts.HookBase
}

func (h *MsgHook) OnPublish(cl *mqtts.Client, pk packets.Packet) (packets.Packet, error) {
	mqtt_log_Printf("OnPublish on the local broker by %v: %v, %v", cl.ID, pk.TopicName, base64.StdEncoding.EncodeToString(pk.Payload))
	if cl.ID != "inline" && !strings.Contains(pk.TopicName, "/inline/") {
		c := clientMap[cl.ID]
		if c.client != nil && c.client.IsConnected() {
			mqtt_log_Printf("relaying to the upstream")
			c.client.Publish(pk.TopicName, pk.FixedHeader.Qos, pk.FixedHeader.Retain, pk.Payload)
		}
	}
	if strings.HasSuffix(pk.TopicName, "/BIRTH") {
		b, err := parseRawMessage(pk.Payload)
		if err == nil {
			c := clientMap[cl.ID]
			c.birth = parseBirthMessage(b)
			clientMap[cl.ID] = c
		} else {
			mqtt_log_Printf("Error while decoding the birth payload: %v", err)
		}
	} else if strings.HasSuffix(pk.TopicName, "/REPLY/params") {
		b, err := parseRawMessage(pk.Payload)
		if err == nil {
			c := clientMap[cl.ID]
			c.params, c.paramsLimits = parseParams(b)
			clientMap[cl.ID] = c
		} else {
			mqtt_log_Printf("Error while decoding the params payload: %v", err)
		}
	} else if strings.HasSuffix(pk.TopicName, "/REPLY/consumptions") {
		b, err := parseConsumptionMessage(pk.Payload)
		if err == nil {
			c := clientMap[cl.ID]
			c.cWh = b
			clientMap[cl.ID] = c
		} else {
			mqtt_log_Printf("Error while decoding the params payload: %v", err)
		}
	} else if strings.HasSuffix(pk.TopicName, "/ErrListRst") {
		b, err := parseRawMessage(pk.Payload)
		if err == nil {
			c := clientMap[cl.ID]
			c.errors = b
			clientMap[cl.ID] = c
		} else {
			mqtt_log_Printf("Error while decoding the params payload: %v", err)
		}
	} else {
		parseRawMessage(pk.Payload)
	}
	return pk, nil
}
func (h *MsgHook) OnSubscribe(cl *mqtts.Client, pk packets.Packet) packets.Packet {
	c := clientMap[cl.ID]
	for _, s := range pk.Filters {
		mqtt_log_Printf("OnSubscribe: %v", s.Filter)
		if c.client != nil && c.client.IsConnected() {
			c.client.Subscribe(s.Filter, s.Qos, nil)
		}
	}
	return pk
}
func (h *MsgHook) Provides(b byte) bool {
	return bytes.Contains([]byte{mqtts.OnPublish, mqtts.OnSubscribe}, []byte{b})
}

func mqttLogic() {
	// Create the new MQTT Server.
	server = mqtts.New(&mqtts.Options{
		InlineClient: true,
	})

	// Authz logic
	ah := &AuthHook{server: server}
	if err := server.AddHook(ah, nil); err != nil {
		log.Fatal(err)
	}

	// Message lgoic
	if err := server.AddHook(new(MsgHook), nil); err != nil {
		log.Fatal(err)
	}

	if Config.Mqtt_broker_clear_listener != "" {
		tcpListener := listeners.NewTCP(listeners.Config{ID: "tcp", Address: Config.Mqtt_broker_clear_listener})
		if err := server.AddListener(tcpListener); err != nil {
			log.Fatal(err)
		}
	}

	if Config.Mqtt_broker_tls_listener != "" {
		cer, err := tls.LoadX509KeyPair(Config.Mqtt_broker_certificate_path, Config.Mqtt_broker_private_key_path)
		if err != nil {
			log.Fatal(err)
		}

		tlsConfig := &tls.Config{Certificates: []tls.Certificate{cer}}
		tlsListener := listeners.NewTCP(listeners.Config{ID: "t1s", Address: Config.Mqtt_broker_tls_listener, TLSConfig: tlsConfig})
		if err := server.AddListener(tlsListener); err != nil {
			log.Fatal(err)
		}
	}

	go func() {
		err := server.Serve()
		if err != nil {
			log.Fatal(err)
		} else {
			mqtt_log_Printf("Starting MQTT listener at %v\n", Config.Mqtt_broker_tls_listener)
		}
	}()

	// Parameter scan
	go func() {
		// Wait for the first client
		for {
			connected := false
			for clientId := range clientMap {
				if clientMap[clientId].client.IsConnected() {
					connected = true
					break
				}
			}
			if connected {
				time.Sleep(time.Duration(2) * time.Second)
				break
			}
			time.Sleep(time.Duration(1) * time.Second)
		}

		for {
			for clientId := range clientMap {
				mqtt_log_Printf("requesting parameters: %v", clientId)
				for _, device := range Config.Devices {
					if device.GwID == clientId {
						if device.Sys == 4 {
							if device.WheType == 6 {
								m, err := getParamMessageRaw([]string{"T_18.0.0", "T_18.0.1", "T_18.0.2", "T_18.0.3", "T_18.0.5", "T_18.1.0", "T_18.1.3",
									"T_18.3.0", "T_18.3.1", "T_18.3.2", "T_18.3.3", "T_18.3.5", "T_18.3.6"})
								if err == nil {

									err := server.Publish("$EDC/ari/"+clientId+"/ar1/GET/Menu/Par", m, false, 0)
									if err != nil {
										mqtt_log_Printf("unable to publish message to read out parameters to %v: %v", clientId, err)
									}
								} else {
									mqtt_log_Printf("unable to build params query: %v", err)
								}
							}
							if device.WheType == 2 {
								m, err := getParamMessageRaw([]string{"T_22.0.0", "T_22.0.1", "T_22.0.2", "T_22.0.3", "T_22.0.4", "T_22.0.5", "T_22.1.0",
									"T_22.1.1", "T_22.1.2", "T_22.1.3", "T_22.1.4", "T_22.2.1", "T_22.2.2", "T_22.3.0", "T_22.3.1", "T_22.3.4", "T_22.3.5",
									"T_22.3.6", "T_22.3.9"})
								if err == nil {

									err := server.Publish("$EDC/ari/"+clientId+"/ar1/GET/Menu/Par", m, false, 0)
									if err != nil {
										mqtt_log_Printf("unable to publish message to read out parameters to %v: %v", clientId, err)
									}
								} else {
									mqtt_log_Printf("unable to build params query: %v", err)
								}
							}
						}
					}
				}
			}

			time.Sleep(time.Duration(Config.Poll_frequency) * time.Second)
		}
	}()

	// Consumption scan
	go func() {
		// Wait for the first client
		for {
			connected := false
			for clientId := range clientMap {
				if clientMap[clientId].client.IsConnected() {
					connected = true
					break
				}
			}
			if connected {
				time.Sleep(time.Duration(10) * time.Second)
				break
			}
			time.Sleep(time.Duration(1) * time.Second)
		}
		for {
			for clientId := range clientMap {
				mqtt_log_Printf("requesting consumptions: %v", clientId)
				for _, device := range Config.Devices {
					if device.GwID == clientId {
						m, err := getConsumptionParamMessageRaw(device.ConsumptionTyp)
						if err == nil {
							err = server.Publish("$EDC/ari/"+clientId+"/ar1/GET/Stat/cWh", m, false, 0)
							if err != nil {
								mqtt_log_Printf("unable to publish message to read out consumptions to %v: %v", clientId, err)
							}
						}
					}
				}
			}

			time.Sleep(time.Duration(Config.Consumption_poll_frequency) * time.Second)
		}
	}()

	// TODO error scan
	// Error scan
	// go func() {
	// 	for {
	// 		for clientId := range clientMap {
	// 			mqtt_log_Printf("requesting parameters: %v", clientId)

	// 			err := server.Publish("ari/"+clientId+"/ar1/Err/ErrListRst", nil, false, 0)
	// 			if err != nil {
	// 				mqtt_log_Printf("unable to publish message to read out parameters to %v: %v", clientId, err)
	// 			}
	// 		}

	// 		time.Sleep(time.Duration(viper.GetInt("poll-frequency")) * time.Second)
	// 	}
	// }()
}
