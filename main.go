package main

import (
	"log"
	"net/url"
	"os"

	"github.com/breez/breez-lnurl/dns"
	"github.com/breez/breez-lnurl/persist"
)

func main() {
	// create the storage and start the server
	storage, err := persist.NewPgStore(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("failed to create postgres store: %v", err)
	}

	externalURL, err := parseURLFromEnv("SERVER_EXTERNAL_URL", "http://localhost:8080")
	if err != nil {
		log.Fatalf("failed to parse external server URL %v", err)
	}

	dnsService := dns.NewNoDns()
	if nameServer := os.Getenv("NAME_SERVER"); nameServer != "" {
		dnsProtocol := os.Getenv("DNS_PROTOCOL")
		tsigKey := os.Getenv("TSIG_KEY")
		tsigSecret := os.Getenv("TSIG_SECRET")
		if len(tsigKey) == 0 || len(tsigSecret) == 0 {
			log.Fatalf("TSIG_KEY and TSIG_SECRET must be set when using DNS")
		}

		dnsService = dns.NewDns(externalURL, nameServer, dnsProtocol, tsigKey, tsigSecret)
	}

	internalURL, err := parseURLFromEnv("SERVER_INTERNAL_URL", "http://localhost:8080")
	if err != nil {
		log.Fatalf("failed to parse internal server URL %v", err)
	}

	NewServer(internalURL, externalURL, storage, dnsService).Serve()
}

func parseURLFromEnv(envKey string, defaultURL string) (*url.URL, error) {
	serverURLStr := os.Getenv(envKey)
	if serverURLStr == "" {
		serverURLStr = defaultURL
	}
	return url.Parse(serverURLStr)
}
