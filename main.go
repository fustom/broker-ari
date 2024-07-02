package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

type config struct {
	Api_debug                    bool
	Api_listener                 string
	Api_password                 string
	Api_username                 string
	Dns_listener                 string
	Dns_resolve_to               string
	Ntp_resolve_to               string
	Mqtt_debug                   bool
	Mqtt_broker_certificate_path string
	Mqtt_broker_clear_listener   string
	Mqtt_broker_private_key_path string
	Mqtt_broker_tls_listener     string
	Mqtt_proxy_upstream          string
	Parser_debug                 bool
	Poll_frequency               int
	Consumption_poll_frequency   int
	Devices                      []Devices
}

type Devices struct {
	GwID              string
	Sys               int
	WheType           int
	WheModelType      int
	Name              string
	ConsumptionTyp    string
	ConsumptionOffset int
}

var Config config

func main() {
	file, err := os.OpenFile("/config/broker-ari.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}

	log.SetOutput(file)

	viper.SetConfigName("config.json")
	viper.SetConfigType("json")
	viper.AddConfigPath("/config")
	viper.ReadInConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		err = viper.Unmarshal(&Config)
		if err != nil {
			log.Fatalf("unable to decode into struct, %v", err)
		}
	})
	viper.WatchConfig()

	err = viper.Unmarshal(&Config)
	if err != nil {
		log.Fatalf("unable to decode into struct, %v", err)
	}

	// Create signals channel to run server until interrupted
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		done <- true
	}()

	mqttLogic()
	apiLogic()
	dnsLogic()

	// Run server until interrupted
	<-done

	// Cleanup
}
