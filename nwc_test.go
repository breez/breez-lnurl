package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/breez/breez-lnurl/cache"
	"github.com/breez/breez-lnurl/nwc"
	"github.com/breez/breez-lnurl/persist"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func TestNwcRegistration(t *testing.T) {
	storage := persist.NewMemoryStore()
	dns := &MockDns{}
	cache := cache.NewCache(time.Minute)

	serverAddress, err := setupServer(storage, dns, cache)
	if err != nil {
		t.Fatalf("Failed to setup server: %v", err)
	}

	hookServerAddress, err := setupHookServer(t)
	if err != nil {
		t.Fatalf("Failed to setup hook server: %v", err)
	}

	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}
	pubkey := hex.EncodeToString(privKey.PubKey().SerializeCompressed())

	relays := []string{"wss://relay.example.com"}
	webhookUrl := fmt.Sprintf("http://%v/callback", hookServerAddress)
	messageToSign := fmt.Sprintf("%v-%v-%v", webhookUrl, pubkey, relays)
	signature, err := signMessage(messageToSign, privKey)
	if err != nil {
		t.Fatalf("Failed to sign message: %v", err)
	}

	request := nwc.RegisterNostrEventsRequest{
		WebhookUrl: webhookUrl,
		AppPubkey:  pubkey,
		Relays:     relays,
		Signature:  *signature,
	}

	payload, _ := json.Marshal(request)
	resp, err := http.Post(fmt.Sprintf("http://%v/nwc/%v", serverAddress, pubkey),
		"application/json", bytes.NewBuffer(payload))
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %v", resp.StatusCode)
	}

	webhook, err := storage.Nwc.Get(context.Background(), pubkey, pubkey)
	if err != nil {
		t.Errorf("Expected webhook to be registered, got error: %v", err)
	}
	if webhook == nil {
		t.Errorf("Expected webhook to be registered")
	}
}

func TestNwcInvalidSignature(t *testing.T) {
	storage := persist.NewMemoryStore()
	dns := &MockDns{}
	cache := cache.NewCache(time.Minute)

	serverAddress, err := setupServer(storage, dns, cache)
	if err != nil {
		t.Fatalf("Failed to setup server: %v", err)
	}

	hookServerAddress, err := setupHookServer(t)
	if err != nil {
		t.Fatalf("Failed to setup hook server: %v", err)
	}

	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}
	pubkey := hex.EncodeToString(privKey.PubKey().SerializeCompressed())

	webhookUrl := fmt.Sprintf("http://%v/callback", hookServerAddress)
	request := nwc.RegisterNostrEventsRequest{
		WebhookUrl: webhookUrl,
		AppPubkey:  pubkey,
		Relays:     []string{"wss://relay.example.com"},
		Signature:  "invalid_signature",
	}

	payload, _ := json.Marshal(request)
	resp, err := http.Post(fmt.Sprintf("http://%v/nwc/%v", serverAddress, pubkey),
		"application/json", bytes.NewBuffer(payload))
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("Expected status code 401, got %v", resp.StatusCode)
	}
}

func TestNwcMultipleRelays(t *testing.T) {
	storage := persist.NewMemoryStore()
	dns := &MockDns{}
	cache := cache.NewCache(time.Minute)

	serverAddress, err := setupServer(storage, dns, cache)
	if err != nil {
		t.Fatalf("Failed to setup server: %v", err)
	}

	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}
	pubkey := hex.EncodeToString(privKey.PubKey().SerializeCompressed())

	relays := []string{"wss://relay1.example.com", "wss://relay2.example.com", "wss://relay3.example.com"}
	webhookUrl := "http://localhost:8080/callback"
	messageToSign := fmt.Sprintf("%v-%v-%v", webhookUrl, pubkey, relays)
	signature, err := signMessage(messageToSign, privKey)
	if err != nil {
		t.Fatalf("Failed to sign message: %v", err)
	}

	request := nwc.RegisterNostrEventsRequest{
		WebhookUrl: webhookUrl,
		AppPubkey:  pubkey,
		Relays:     relays,
		Signature:  *signature,
	}

	payload, _ := json.Marshal(request)
	resp, err := http.Post(fmt.Sprintf("http://%v/nwc/%v", serverAddress, pubkey),
		"application/json", bytes.NewBuffer(payload))
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %v", resp.StatusCode)
	}

	webhook, err := storage.Nwc.Get(context.Background(), pubkey, pubkey)
	if err != nil {
		t.Errorf("Expected webhook to be registered, got error: %v", err)
	}
	if webhook == nil {
		t.Errorf("Expected webhook to be registered")
	}
	if webhook != nil && len(webhook.Relays) != 3 {
		t.Errorf("Expected 3 relays, got %d", len(webhook.Relays))
	}

	if !compareRelaySlices(webhook.Relays, relays) {
		t.Errorf("Expected relays %v, got %v", relays, webhook.Relays)
	}
}

func TestNwcRegistrationOverwrite(t *testing.T) {
	storage := persist.NewMemoryStore()
	dns := &MockDns{}
	cache := cache.NewCache(time.Minute)

	serverAddress, err := setupServer(storage, dns, cache)
	if err != nil {
		t.Fatalf("Failed to setup server: %v", err)
	}

	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}
	pubkey := hex.EncodeToString(privKey.PubKey().SerializeCompressed())

	// First registration
	relays1 := []string{"wss://relay1.example.com"}
	webhookUrl1 := "http://localhost:8080/callback1"
	messageToSign1 := fmt.Sprintf("%v-%v-%v", webhookUrl1, pubkey, relays1)
	signature1, err := signMessage(messageToSign1, privKey)
	if err != nil {
		t.Fatalf("Failed to sign message: %v", err)
	}

	request1 := nwc.RegisterNostrEventsRequest{
		WebhookUrl: webhookUrl1,
		AppPubkey:  pubkey,
		Relays:     relays1,
		Signature:  *signature1,
	}

	payload1, _ := json.Marshal(request1)
	resp1, err := http.Post(fmt.Sprintf("http://%v/nwc/%v", serverAddress, pubkey),
		"application/json", bytes.NewBuffer(payload1))
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if resp1.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %v", resp1.StatusCode)
	}

	// Second registration (should overwrite first)
	relays2 := []string{"wss://relay2.example.com"}
	webhookUrl2 := "http://localhost:8080/callback2"
	messageToSign2 := fmt.Sprintf("%v-%v-%v", webhookUrl2, pubkey, relays2)
	signature2, err := signMessage(messageToSign2, privKey)
	if err != nil {
		t.Fatalf("Failed to sign message: %v", err)
	}

	request2 := nwc.RegisterNostrEventsRequest{
		WebhookUrl: webhookUrl2,
		AppPubkey:  pubkey,
		Relays:     relays2,
		Signature:  *signature2,
	}

	payload2, _ := json.Marshal(request2)
	resp2, err := http.Post(fmt.Sprintf("http://%v/nwc/%v", serverAddress, pubkey),
		"application/json", bytes.NewBuffer(payload2))
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if resp2.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %v", resp2.StatusCode)
	}

	// Verify second registration overwrote first
	webhook, err := storage.Nwc.Get(context.Background(), pubkey, pubkey)
	if err != nil {
		t.Errorf("Expected webhook to be registered, got error: %v", err)
	}
	if webhook == nil {
		t.Errorf("Expected webhook to be registered")
	}
	if webhook != nil && webhook.Url != "http://localhost:8080/callback2" {
		t.Errorf("Expected webhook URL to be overwritten, got %v", webhook.Url)
	}
	if webhook != nil && len(webhook.Relays) != 1 {
		t.Errorf("Expected 1 relay, got %d", len(webhook.Relays))
	}
}

func compareRelaySlices(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}

	actualMap := make(map[string]bool)
	expectedMap := make(map[string]bool)

	for _, relay := range actual {
		actualMap[relay] = true
	}
	for _, relay := range expected {
		expectedMap[relay] = true
	}

	for relay := range actualMap {
		if !expectedMap[relay] {
			return false
		}
	}
	for relay := range expectedMap {
		if !actualMap[relay] {
			return false
		}
	}

	return true
}
