package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	flag.Parse()

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
