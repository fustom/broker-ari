package main

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"flag"
	"log"
	"strings"
	"time"

	mqttc "github.com/eclipse/paho.mqtt.golang"
	mqtts "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
)

const (
	CRT_PATH = "/config/broker-ari.everyware-cloud.com.crt"
	KEY_PATH = "/config/broker-ari.everyware-cloud.com.key"

	MQTT_UPSTREAM_BROKER_ADDR = "ssl://broker-ari.everyware-cloud.com:8883"
)

var (
	pollFrequency = flag.Int("poll-frequency", 60, "Frequency (in seconds) of polling the heaters")

	mqttBrokerClearListener = flag.String("mqtt-broker-clear-listener", "", "Listener address (e.g. :1883) of the cleartext MQTT broker")
	mqttBrokerTlsListener   = flag.String("mqtt-broker-tls-listener", ":8883", "Listener address (e.g. :8883) of the cleartext MQTT broker")
	certificatePath         = flag.String("mqtt-broker-certificate-path", CRT_PATH, "Path to the certificate for the TLS listener of the MQTT broker")
	privateKeyPath          = flag.String("mqtt-broker-private-key-path", KEY_PATH, "Path to the private key for the TLS listener of the MQTT broker")

	mqttProxyUpstream = flag.String("mqtt-proxy-upstream", MQTT_UPSTREAM_BROKER_ADDR, "Address of the official upstream broker. Use an empty string to have a local only experience")

	clientMap = map[string]mqttClient{}
	server    *mqtts.Server
)

type mqttClient struct {
	client mqttc.Client
	birth  map[string]any
	params map[string]any
}

type AuthHook struct {
	auth.Hook
	server *mqtts.Server
}

func (h *AuthHook) OnPacketRead(cl *mqtts.Client, pk packets.Packet) (packets.Packet, error) {
	// log.Printf("OnPacketRead:  %+v", pk)
	if pk.Connect.WillFlag && len(pk.Connect.WillPayload) == 0 {
		// they violate [MQTT-3.1.2-9]
		log.Printf("Patching WILL payload")
		pk.Connect.WillPayload = []byte{0}
	}
	return pk, nil
}
func (h *AuthHook) OnACLCheck(cl *mqtts.Client, topic string, write bool) bool {
	log.Printf("OnACLCheck: topic: %v, write: %v", topic, write)
	return true
}
func (h *AuthHook) OnConnectAuthenticate(cl *mqtts.Client, pk packets.Packet) bool {
	// log.Printf("OnConnectAuthenticate: %v connect params: %+v", cl.ID, pk.Connect)
	log.Printf("OnConnectAuthenticate: %v, username: %v, password: %+v", cl.ID, string(pk.Connect.Username), string(pk.Connect.Password))

	if *mqttProxyUpstream == "" {
		// proxying is disabled
		return true
	}

	opts := mqttc.NewClientOptions()
	opts.AddBroker(*mqttProxyUpstream)
	opts.SetKeepAlive(0xeb)
	opts.SetBinaryWill(pk.Connect.WillTopic, []byte{}, pk.Connect.WillQos, pk.Connect.WillRetain)
	opts.SetAutoReconnect(true)
	opts.SetClientID(cl.ID)
	opts.SetUsername(string(pk.Connect.Username))
	opts.SetPassword(string(pk.Connect.Password))
	opts.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	opts.SetDefaultPublishHandler(func(client mqttc.Client, msg mqttc.Message) {
		log.Printf("OnPublish from the upstream: %s, topic: %s payload: %s", cl.ID, msg.Topic(),
			base64.StdEncoding.EncodeToString(msg.Payload()))
		h.server.Publish(msg.Topic(), msg.Payload(), msg.Retained(), msg.Qos())
	})
	log.Printf("connecting to the upstream mqtt broker: %v, %v", cl.ID, *mqttProxyUpstream)
	client := mqttc.NewClient(opts)
	if token := client.Connect(); token.WaitTimeout(5*time.Second) && token.Error() != nil {
		log.Printf("Unable to connect to the upstream mqtt broker: %v", token.Error())
		return false
	}

	clientMap[cl.ID] = mqttClient{
		client: client,
	}

	return true
}
func (h *AuthHook) OnDisconnect(cl *mqtts.Client, err error, expire bool) {
	log.Printf("OnDisconnect on the local broker: %v: %v", cl.ID, err)
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
	log.Printf("OnPublish on the local broker by %v: %v, %v", cl.ID, pk.TopicName, base64.StdEncoding.EncodeToString(pk.Payload))
	if cl.ID != "inline" && !strings.Contains(pk.TopicName, "/inline/") {
		c := clientMap[cl.ID]
		if c.client != nil && c.client.IsConnected() {
			log.Printf("relaying to the upstream")
			c.client.Publish(pk.TopicName, pk.FixedHeader.Qos, pk.FixedHeader.Retain, pk.Payload)
		}
	}
	if strings.HasSuffix(pk.TopicName, "/BIRTH") {
		b, _, err := parseRawMessageToMap(pk.Payload)
		if err == nil {
			c := clientMap[cl.ID]
			c.birth = b
			clientMap[cl.ID] = c
		} else {
			log.Printf("Error while decoding the birth payload: %v", err)
		}
	}
	if strings.HasSuffix(pk.TopicName, "/REPLY/params") {
		b, _, err := parseRawMessageToMap(pk.Payload)
		if err == nil {
			c := clientMap[cl.ID]
			c.params = b
			clientMap[cl.ID] = c
		} else {
			log.Printf("Error while decoding the params payload: %v", err)
		}
	}
	return pk, nil
}
func (h *MsgHook) OnSubscribe(cl *mqtts.Client, pk packets.Packet) packets.Packet {
	c := clientMap[cl.ID]
	for _, s := range pk.Filters {
		log.Printf("OnSubscribe: %v", s.Filter)
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

	if *mqttBrokerClearListener != "" {
		tcpListener := listeners.NewTCP(listeners.Config{ID: "tcp", Address: *mqttBrokerClearListener})
		if err := server.AddListener(tcpListener); err != nil {
			log.Fatal(err)
		}
	}

	if *mqttBrokerTlsListener != "" {
		cer, err := tls.LoadX509KeyPair(*certificatePath, *privateKeyPath)
		if err != nil {
			log.Fatal(err)
		}

		tlsConfig := &tls.Config{Certificates: []tls.Certificate{cer}}
		tlsListener := listeners.NewTCP(listeners.Config{ID: "t1s", Address: *mqttBrokerTlsListener, TLSConfig: tlsConfig})
		if err := server.AddListener(tlsListener); err != nil {
			log.Fatal(err)
		}
	}

	go func() {
		err := server.Serve()
		if err != nil {
			log.Fatal(err)
		}
	}()

	go func() {
		for {
			m, err := getParamMessageRaw([]string{"T_22.0.0", "T_22.0.3", "T_22.3.0", "T_22.3.4", "T_22.3.6", "T_22.3.1", "T_22.1.3", "T_22.1.0", "T_22.3.9"})
			if err == nil {

				for clientId, _ := range clientMap {
					log.Printf("requesting parameters: %v", clientId)

					err := server.Publish("$EDC/ari/"+clientId+"/ar1/GET/Menu/Par", m, false, 0)
					if err != nil {
						log.Printf("unable to publish message to read out parameters to %v: %v", clientId, err)
					}
				}
			} else {
				log.Printf("unable to build params query: %v", err)
			}

			time.Sleep(time.Duration(*pollFrequency) * time.Second)
		}
	}()

}
