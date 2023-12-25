package main

import (
	"log"
	"net/url"
	"os"

	"github.com/breez/breez-lnurl/persist"
)

func main() {
	// create the storage and start the server
	storage := &persist.MemoryStore{}

	// parse external URL
	externalURL, err := parseURLFromEnv("SERVER_EXTERNAL_URL", "http://localhost:8080")
	if err != nil {
		log.Fatalf("failed to parse server URL %v", err)
	}

	internalURL, err := parseURLFromEnv("SERVER_INTERNAL_URL", "http://localhost:8080")
	if err != nil {
		log.Fatalf("failed to parse server URL %v", err)
	}

	proxyEndpoints := []string{
		"lnurlpay",         // The initial endpoint for the node lnurlpay
		"lnurlpay_invoice", // The lnurlpay endpoint to get the invoice
	}
	NewServer(internalURL, externalURL, storage, proxyEndpoints).Serve()
}

func parseURLFromEnv(envKey string, defaultURL string) (*url.URL, error) {
	serverURLStr := os.Getenv(envKey)
	if serverURLStr == "" {
		serverURLStr = defaultURL
	}
	serverURL, err := url.Parse(serverURLStr)
	if err != nil {
		log.Fatalf("failed to parse server URL %v", err)
	}
	return serverURL, nil
}
