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

	"github.com/breez/breez-lnurl/channel"
	"github.com/breez/breez-lnurl/lnurl"
	"github.com/breez/breez-lnurl/persist"
	"github.com/breez/breez-lnurl/webhook"
	"github.com/breez/lspd/lightning"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/gorilla/mux"
	"github.com/tv42/zbase32"
)

const (
	hookKey           = "456"
	serverAddress     = "localhost:8080"
	hookServerAddress = "localhost:8085"
	testFeature       = "testFeature"
	testEndpoint      = "testEndpoint"
)

func setupServer(storage persist.Store) {
	serverURL, err := url.Parse(fmt.Sprintf("http://%v", serverAddress))
	if err != nil {
		log.Fatalf("failed to parse server URL %v", err)
	}
	server := NewServer(serverURL, serverURL, storage)
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
		response, err := http.Post(payload.Data.CallbackURL, "application/json", bytes.NewBuffer([]byte(`{"status": "ok"}`)))
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
	url := fmt.Sprintf("http://%v/callback", hookServerAddress)
	time := time.Now().Unix()
	messgeToSign := fmt.Sprintf("%v-%v-%v", time, hookKey, url)
	msg := append(lightning.SignedMsgPrefix, []byte(messgeToSign)...)
	first := sha256.Sum256([]byte(msg))
	second := sha256.Sum256(first[:])
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Errorf("failed to generate private key %v", err)
	}
	pubkey := privKey.PubKey()
	sig, err := ecdsa.SignCompact(privKey, second[:], true)
	if err != nil {
		t.Errorf("failed to sign signature %v", err)
	}
	serializedPubkey := hex.EncodeToString(pubkey.SerializeCompressed())
	addWebhookPayload, _ := json.Marshal(webhook.AddWebhookRequest{
		Time:      time,
		HookKey:   hookKey,
		Url:       url,
		Signature: zbase32.EncodeToString(sig),
	})

	httpRes, err := http.Post(fmt.Sprintf("http://%v/webhooks/%v", serverAddress, serializedPubkey), "application/json", bytes.NewBuffer(addWebhookPayload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if httpRes.StatusCode != 200 {
		t.Errorf("expected status code 200, got %v", httpRes.StatusCode)
	}

	h := sha256.New()
	if _, err := h.Write([]byte(hookKey)); err != nil {
		t.Errorf("failed hash key %v", err)
	}
	keyHash := hex.EncodeToString(h.Sum(nil))
	webhook, _ := storage.Get(context.Background(), serializedPubkey, keyHash)
	if webhook == nil {
		t.Errorf("expected webhook to be registered")
	}

	// Test lnurlpay info endpoint
	u := fmt.Sprintf("http://%v/lnurlpay/%v/%v", serverAddress, serializedPubkey, keyHash)
	proxyRes, err := http.Get(u)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if proxyRes.StatusCode != 200 {
		t.Errorf("expected status code 200, got %v", proxyRes.StatusCode)
	}

	// Test lnurlpay info endpoint with invalid amount
	u = fmt.Sprintf("http://%v/lnurlpay/%v/%v/invoice", serverAddress, serializedPubkey, keyHash)
	response := testInvoiceRequest(t, u, serializedPubkey, keyHash)
	if response.Status != "ERROR" {
		t.Errorf("Got error from lnurlpay invoice response %v", response.Status)
	}

	// Test lnurlpay info endpoint with valid amount
	u = fmt.Sprintf("http://%v/lnurlpay/%v/%v/invoice?amount=100", serverAddress, serializedPubkey, keyHash)
	response = testInvoiceRequest(t, u, serializedPubkey, keyHash)
	if response.Status == "ERROR" {
		t.Errorf("Got error from lnurlpay invoice response %v", response.Status)
	}
}

func testInvoiceRequest(t *testing.T, url string, serializedPubkey string, keyHash string) lnurl.LnurlPayStatus {
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
