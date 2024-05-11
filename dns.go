package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/miekg/dns"
)

var (
	dnsResolveTo = flag.String("dns-resolve-to", "", "When configured, a DNS server will be started that resolves A record lookups to this IP address")
	dnsListener  = flag.String("dns-listener", ":53", "Address to listen on when --dns-resolve-to is configured")
)

func parseQuery(m *dns.Msg) {
	for _, q := range m.Question {
		switch q.Qtype {
		case dns.TypeA:
			log.Printf("Query for %s\n", q.Name)
			if q.Name == "broker-ari.everyware-cloud.com." {
				rr, err := dns.NewRR(fmt.Sprintf("%s A %s", q.Name, *dnsResolveTo))
				if err == nil {
					m.Answer = append(m.Answer, rr)
				}
			}
		}
	}
}

func handleDnsRequest(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false

	switch r.Opcode {
	case dns.OpcodeQuery:
		parseQuery(m)
	}

	w.WriteMsg(m)
}

func dnsLogic() {
	if *dnsResolveTo == "" {
		return
	}

	// attach request handler func
	dns.HandleFunc(".", handleDnsRequest)

	// start server
	server := &dns.Server{Addr: *dnsListener, Net: "udp"}
	log.Printf("Starting DNS listener at %v\n", *dnsListener)

	go func() {
		err := server.ListenAndServe()
		if err != nil {
			log.Fatalf("Failed to start server: %s\n ", err.Error())
		}
	}()

}
