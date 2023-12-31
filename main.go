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
		log.Fatalf("failed to parse external server URL %v", err)
	}

	internalURL, err := parseURLFromEnv("SERVER_INTERNAL_URL", "http://localhost:8080")
	if err != nil {
		log.Fatalf("failed to parse internal server URL %v", err)
	}

	NewServer(internalURL, externalURL, storage).Serve()
}

func parseURLFromEnv(envKey string, defaultURL string) (*url.URL, error) {
	serverURLStr := os.Getenv(envKey)
	if serverURLStr == "" {
		serverURLStr = defaultURL
	}
	return url.Parse(serverURLStr)
}
