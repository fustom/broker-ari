package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/miekg/dns"
)

func parseQuery(m *dns.Msg) {
	for _, q := range m.Question {
		switch q.Qtype {
		case dns.TypeA:
			log.Printf("Query for %s\n", q.Name)
			if q.Name == "broker-ari.everyware-cloud.com." {
				rr, err := dns.NewRR(fmt.Sprintf("%s A %s", q.Name, Config.Dns_resolve_to))
				if err == nil {
					m.Answer = append(m.Answer, rr)
				}
			}
			if strings.Contains(q.Name, "pool.ntp.org.") {
				rr, err := dns.NewRR(fmt.Sprintf("%s A %s", q.Name, Config.Ntp_resolve_to))
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
	if Config.Dns_resolve_to == "" {
		return
	}

	// attach request handler func
	dns.HandleFunc(".", handleDnsRequest)

	// start server
	server := &dns.Server{Addr: Config.Dns_listener, Net: "udp"}
	log.Printf("Starting DNS listener at %v\n", Config.Dns_listener)

	go func() {
		err := server.ListenAndServe()
		if err != nil {
			log.Fatalf("Failed to start server: %s\n ", err.Error())
		}
	}()

}
