package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"testing"

	"github.com/breez/breez-lnurl/channel"
	"github.com/breez/breez-lnurl/persist"
	"github.com/breez/breez-lnurl/webhook"
	"github.com/gorilla/mux"
)

const (
	pubkey            = "123"
	hookKey           = "456"
	serverAddress     = "localhost:8080"
	hookServerAddress = "localhost:8085"
	testEndpoint      = "test"
)

func setupServer(storage persist.Store) {
	serverURL, err := url.Parse(fmt.Sprintf("http://%v", serverAddress))
	if err != nil {
		log.Fatalf("failed to parse server URL %v", err)
	}
	server := NewServer(serverURL, serverURL, storage, []string{testEndpoint})
	go func() {
		persist.NewCleanupService(storage).Start(context.Background())
	}()
	go func() {
		if err := server.Serve(); err != nil {
			fmt.Printf("server.Serve error: %v", err)
		}
	}()
}

func setupHookServer(t *testing.T) {
	callbackRouter := mux.NewRouter()
	callbackRouter.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		allBody, _ := io.ReadAll(r.Body)
		var payload channel.WebhookChannelChannelPayload
		if err := json.Unmarshal(allBody, &payload); err != nil {
			t.Errorf("unmarshal proxy payload, expected no error, got %v", err)
		}
		response, err := http.Post(payload.CallbackURL, "application/json", bytes.NewBuffer([]byte(`{"status": "ok"}`)))
		if err != nil {
			t.Errorf("failed to invoke hook callback %v", err)
		}
		if response.StatusCode != 200 {
			t.Errorf("expected status code 200, got %v", response.StatusCode)
		}
	}).Methods("POST")
	go func() {
		if err := http.ListenAndServe(hookServerAddress, callbackRouter); err != nil {
			t.Errorf("failed to start hook server %v", err)
		}
	}()
}

func TestRegisterWebhook(t *testing.T) {
	storage := &persist.MemoryStore{}
	setupServer(storage)
	setupHookServer(t)

	// Test adding webhook
	addWebhookPayload, _ := json.Marshal(webhook.AddWebhookRequest{
		HookID:    hookKey,
		Url:       fmt.Sprintf("http://%v/callback", hookServerAddress),
		Signature: "",
	})

	httpRes, err := http.Post(fmt.Sprintf("http://%v/webhooks/%v", serverAddress, pubkey), "application/json", bytes.NewBuffer(addWebhookPayload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if httpRes.StatusCode != 200 {
		t.Errorf("expected status code 200, got %v", httpRes.StatusCode)
	}
	webhook, _ := storage.Get(context.Background(), pubkey, hookKey)
	if webhook == nil {
		t.Errorf("expected webhook to be registered")
	}

	// Test proxy endpoint
	proxyRes, err := http.Get(fmt.Sprintf("http://%v/%v/%v/%v", serverAddress, pubkey, hookKey, testEndpoint))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if proxyRes.StatusCode != 200 {
		t.Errorf("expected status code 200, got %v", proxyRes.StatusCode)
	}
}
