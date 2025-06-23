package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/breez/breez-lnurl/cache"
	"github.com/breez/breez-lnurl/channel"
	"github.com/breez/breez-lnurl/dns"
	"github.com/breez/breez-lnurl/lnurl"
	"github.com/breez/breez-lnurl/persist"
	"github.com/breez/lspd/lightning"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/gorilla/mux"
	"github.com/tv42/zbase32"
)

const (
	serverAddress     = "localhost:8080"
	hookServerAddress = "localhost:8085"
	testFeature       = "testFeature"
	testEndpoint      = "testEndpoint"
)

func setupServer(storage persist.Store, dns dns.DnsService, cache cache.CacheService) {
	serverURL, err := url.Parse(fmt.Sprintf("http://%v", serverAddress))
	if err != nil {
		log.Fatalf("failed to parse server URL %v", err)
	}
	server := NewServer(serverURL, serverURL, storage, dns, cache)
	go func() {
		persist.NewCleanupService(storage).Start(context.Background())
	}()
	go func() {
		if err := server.Serve(); err != nil {
			log.Printf("server.Serve error: %v", err)
		}
	}()
}

func setupHookServer(t *testing.T) {
	callbackRouter := mux.NewRouter()
	callbackRouter.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		allBody, _ := io.ReadAll(r.Body)
		var payload channel.WebhookMessage
		if err := json.Unmarshal(allBody, &payload); err != nil {
			t.Errorf("unmarshal proxy payload, expected no error, got %v", err)
		}
		replyURL, ok := payload.Data["reply_url"].(string)
		if !ok {
			t.Errorf("failed to extract reply_url %+v", payload)
		}
		response, err := http.Post(replyURL, "application/json", bytes.NewBuffer([]byte(`{"status": "ok"}`)))
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
	dns := &dns.NoDns{}
	cache := cache.NewCache(time.Minute)
	setupServer(storage, dns, cache)
	setupHookServer(t)

	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Errorf("failed to generate private key %v", err)
	}
	pubkey := privKey.PubKey()
	serializedPubkey := hex.EncodeToString(pubkey.SerializeCompressed())

	// Test adding webhook
	url := fmt.Sprintf("http://%v/callback", hookServerAddress)
	time := time.Now().Unix()
	signature, err := signMessage(fmt.Sprintf("%v-%v", time, url), privKey)
	if err != nil {
		t.Errorf("failed to sign signature %v", err)
	}
	addWebhookPayload, _ := json.Marshal(lnurl.RegisterLnurlPayRequest{
		Time:       time,
		WebhookUrl: url,
		Signature:  *signature,
	})

	httpRes, err := http.Post(fmt.Sprintf("http://%v/lnurlpay/%v", serverAddress, serializedPubkey), "application/json", bytes.NewBuffer(addWebhookPayload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if httpRes.StatusCode != 200 {
		t.Errorf("expected status code 200, got %v", httpRes.StatusCode)
	}

	webhook, _ := storage.GetLastUpdated(context.Background(), serializedPubkey)
	if webhook == nil {
		t.Errorf("expected webhook to be registered")
	}

	// Test recovering
	recoverPayload, _ := json.Marshal(lnurl.UnregisterRecoverLnurlPayRequest{
		Time:       time,
		WebhookUrl: url,
		Signature:  *signature,
	})
	proxyRes, err := http.Post(fmt.Sprintf("http://%v/lnurlpay/%v/recover", serverAddress, serializedPubkey), "application/json", bytes.NewBuffer(recoverPayload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if proxyRes.StatusCode != 200 {
		t.Errorf("expected status code 200, got %v", proxyRes.StatusCode)
	}

	// Test lnurlpay info endpoint
	u := fmt.Sprintf("http://%v/lnurlp/%v", serverAddress, serializedPubkey)
	proxyRes, err = http.Get(u)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if proxyRes.StatusCode != 200 {
		t.Errorf("expected status code 200, got %v", proxyRes.StatusCode)
	}

	// Test lnurlpay info endpoint with invalid amount
	u = fmt.Sprintf("http://%v/lnurlpay/%v/invoice", serverAddress, serializedPubkey)
	response := testInvoiceRequest(t, u)
	if response.Status != "ERROR" {
		t.Errorf("Got error from lnurlpay invoice response %v", response.Status)
	}

	// Test lnurlpay info endpoint with valid amount
	u = fmt.Sprintf("http://%v/lnurlpay/%v/invoice?amount=100", serverAddress, serializedPubkey)
	response = testInvoiceRequest(t, u)
	if response.Status == "ERROR" {
		t.Errorf("Got error from lnurlpay invoice response %v", response.Status)
	}
}

func TestRegisterWebhookWithUsername(t *testing.T) {
	storage := &persist.MemoryStore{}
	dns := &dns.NoDns{}
	cache := cache.NewCache(time.Minute)
	setupServer(storage, dns, cache)
	setupHookServer(t)

	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Errorf("failed to generate private key %v", err)
	}
	pubkey := privKey.PubKey()
	serializedPubkey := hex.EncodeToString(pubkey.SerializeCompressed())

	// Test adding webhook
	url := fmt.Sprintf("http://%v/callback", hookServerAddress)
	time := time.Now().Unix()
	username := "testuser"
	signature, err := signMessage(fmt.Sprintf("%v-%v-%v", time, url, username), privKey)
	if err != nil {
		t.Errorf("failed to sign signature %v", err)
	}
	addWebhookPayload, _ := json.Marshal(lnurl.RegisterLnurlPayRequest{
		Time:       time,
		WebhookUrl: url,
		Username:   &username,
		Signature:  *signature,
	})

	httpRes, err := http.Post(fmt.Sprintf("http://%v/lnurlpay/%v", serverAddress, serializedPubkey), "application/json", bytes.NewBuffer(addWebhookPayload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if httpRes.StatusCode != 200 {
		t.Errorf("expected status code 200, got %v", httpRes.StatusCode)
	}

	webhook, _ := storage.GetLastUpdated(context.Background(), serializedPubkey)
	if webhook == nil {
		t.Errorf("expected webhook to be registered")
	}

	// Test recovering
	signature, err = signMessage(fmt.Sprintf("%v-%v", time, url), privKey)
	if err != nil {
		t.Errorf("failed to sign signature %v", err)
	}
	recoverPayload, _ := json.Marshal(lnurl.UnregisterRecoverLnurlPayRequest{
		Time:       time,
		WebhookUrl: url,
		Signature:  *signature,
	})
	proxyRes, err := http.Post(fmt.Sprintf("http://%v/lnurlpay/%v/recover", serverAddress, serializedPubkey), "application/json", bytes.NewBuffer(recoverPayload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if proxyRes.StatusCode != 200 {
		t.Errorf("expected status code 200, got %v", proxyRes.StatusCode)
	}

	// Test lnurlpay info endpoint
	u := fmt.Sprintf("http://%v/.well-known/lnurlp/%v", serverAddress, username)
	proxyRes, err = http.Get(u)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if proxyRes.StatusCode != 200 {
		t.Errorf("expected status code 200, got %v", proxyRes.StatusCode)
	}

	// Test lnurlpay info endpoint with invalid amount
	u = fmt.Sprintf("http://%v/lnurlpay/%v/invoice", serverAddress, username)
	response := testInvoiceRequest(t, u)
	if response.Status != "ERROR" {
		t.Errorf("Got error from lnurlpay invoice response %v", response.Status)
	}

	// Test lnurlpay info endpoint with valid amount
	u = fmt.Sprintf("http://%v/lnurlpay/%v/invoice?amount=100", serverAddress, username)
	response = testInvoiceRequest(t, u)
	if response.Status == "ERROR" {
		t.Errorf("Got error from lnurlpay invoice response %v", response.Status)
	}
}

func TestRegisterWebhookWithOffer(t *testing.T) {
	storage := &persist.MemoryStore{}
	dns := &dns.NoDns{}
	cache := cache.NewCache(time.Minute)
	setupServer(storage, dns, cache)
	setupHookServer(t)

	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Errorf("failed to generate private key %v", err)
	}
	pubkey := privKey.PubKey()
	serializedPubkey := hex.EncodeToString(pubkey.SerializeCompressed())

	// Test adding webhook
	url := fmt.Sprintf("http://%v/callback", hookServerAddress)
	time := time.Now().Unix()
	username := "testuser"
	offer := "lno1zzfq9ktw4h4r67qpq3zf4jjujdrpeenuz4jw9cwhxgjl5e7a8wvh5cqcqvet65ahjawgr0r0uk0xznn0d5hrlpn2pqkqpeauwd4lxn33kjha7qgz4g9uzme8aakpehdzgel76lne3sswk6ducu6ygnsh8d87fqah39psqtqweqrf5actfuucvmmlt3k6snksj9dhsgvscj3aa2prf3p386q7p9kzhek7n0aspfmzxpps793pq0kufnlevx9qtyem0tq5g5lym8xt6zcve2kgqe5wv3gf9fcqkmt2z"
	signature, err := signMessage(fmt.Sprintf("%v-%v-%v-%v", time, url, username, offer), privKey)
	if err != nil {
		t.Errorf("failed to sign signature %v", err)
	}
	addWebhookPayload, _ := json.Marshal(lnurl.RegisterLnurlPayRequest{
		Time:       time,
		WebhookUrl: url,
		Username:   &username,
		Offer:      &offer,
		Signature:  *signature,
	})

	httpRes, err := http.Post(fmt.Sprintf("http://%v/lnurlpay/%v", serverAddress, serializedPubkey), "application/json", bytes.NewBuffer(addWebhookPayload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if httpRes.StatusCode != 200 {
		t.Errorf("expected status code 200, got %v", httpRes.StatusCode)
	}

	webhook, _ := storage.GetLastUpdated(context.Background(), serializedPubkey)
	if webhook == nil {
		t.Errorf("expected webhook to be registered")
	}
	if webhook != nil && webhook.Offer == nil {
		t.Errorf("expected webhook to have offer")
	}
}

func testInvoiceRequest(t *testing.T, url string) lnurl.LnurlPayStatus {
	proxyRes, err := http.Get(url)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if proxyRes.StatusCode != 200 {
		t.Errorf("expected status code 200, got %v", proxyRes.StatusCode)
	}
	body, err := io.ReadAll(proxyRes.Body)
	if err != nil {
		t.Errorf("failed to read lnurlpay invoice response body %v", err)
	}
	var response lnurl.LnurlPayStatus
	if err := json.Unmarshal(body, &response); err != nil {
		t.Errorf("failed to unmarhsal lnurlpay invoice response %v", err)
	}
	return response
}

func signMessage(messgeToSign string, privKey *secp256k1.PrivateKey) (*string, error) {
	msg := append(lightning.SignedMsgPrefix, []byte(messgeToSign)...)
	first := sha256.Sum256([]byte(msg))
	second := sha256.Sum256(first[:])
	sig, err := ecdsa.SignCompact(privKey, second[:], true)
	if err != nil {
		return nil, err
	}
	signature := zbase32.EncodeToString(sig)
	return &signature, nil
}
