package webhook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/breez/breez-lnurl/channel"
	"github.com/breez/breez-lnurl/persist"
	"github.com/breez/lspd/lightning"
	"github.com/gorilla/mux"
)

type InvokeWebhookRequest struct {
	Template string          `json:"template"`
	Data     json.RawMessage `json:"data"`
}

type AddWebhookRequest struct {
	Time      int64  `json:"time"`
	HookKey   string `json:"hook_key"`
	Url       string `json:"url"`
	Signature string `json:"signature"`
}

func (w *AddWebhookRequest) Verify(pubkey string) error {
	messgeToVerify := fmt.Sprintf("%v-%v-%v", w.Time, w.HookKey, w.Url)
	verifiedPubkey, err := lightning.VerifyMessage([]byte(messgeToVerify), w.Signature)
	if err != nil {
		return err
	}
	if pubkey != hex.EncodeToString(verifiedPubkey.SerializeCompressed()) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

type RemoveWebhookRequest struct {
	Time      int64  `json:"time"`
	HookKey   string `json:"hook_key"`
	Signature string `json:"signature"`
}

func (w *RemoveWebhookRequest) Verify(pubkey string) error {
	messgeToVerify := fmt.Sprintf("%v-%v", w.Time, w.HookKey)
	verifiedPubkey, err := lightning.VerifyMessage([]byte(messgeToVerify), w.Signature)
	if err != nil {
		return err
	}
	if pubkey != hex.EncodeToString(verifiedPubkey.SerializeCompressed()) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

// The WebhooksRouter is responsible for handling the requests for the webhook endpoints.
// It is currently handling two endpoints:
// 1. Set a webhook for a specific node id and key
// 2. Invoke a webhook for a node id
type WebhooksRouter struct {
	store   persist.Store
	channel channel.WebhookChannel
}

func NewWebhookRouter(rootRouter *mux.Router, store persist.Store, channel channel.WebhookChannel) *WebhooksRouter {
	webhookRouter := &WebhooksRouter{
		store:   store,
		channel: channel,
	}
	// Set webhook for a specific key
	rootRouter.HandleFunc("/webhooks/{pubkey}", webhookRouter.set).Methods("POST")
	// Delete webhook for a specific key
	rootRouter.HandleFunc("/webhooks/{pubkey}", webhookRouter.Remove).Methods("DELETE")

	return webhookRouter
}

/*
Set adds a webhook for a given pubkey and a unique identifier.
The key enables the caller to replace existing hook without deleting it.
*/
func (s *WebhooksRouter) set(w http.ResponseWriter, r *http.Request) {
	var addRequest AddWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&addRequest); err != nil {
		log.Printf("json.NewDecoder.Decode error: %v", err)
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	params := mux.Vars(r)
	pubkey, ok := params["pubkey"]
	if !ok {
		http.Error(w, "invalid pubkey", http.StatusBadRequest)
		return
	}

	if err := addRequest.Verify(pubkey); err != nil {
		log.Printf("failed to verify webhook request: %v", err)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	h := sha256.New()
	h.Write([]byte(addRequest.HookKey))

	err := s.store.Set(context.Background(), persist.Webhook{
		Pubkey:      pubkey,
		Url:         addRequest.Url,
		HookKeyHash: hex.EncodeToString(h.Sum(nil)),
	})

	if err != nil {
		log.Printf(
			"failed to add webhook for %x for notifications on url %s: %v",
			pubkey,
			addRequest.Url,
			err,
		)

		w.WriteHeader(http.StatusInternalServerError)
	}
	w.WriteHeader(http.StatusOK)
}

/*
Remove deletes a webhook for a given pubkey and a unique identifier.
*/
func (s *WebhooksRouter) Remove(w http.ResponseWriter, r *http.Request) {
	var removeRequest RemoveWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&removeRequest); err != nil {
		log.Printf("json.NewDecoder.Decode error: %v", err)
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	params := mux.Vars(r)
	pubkey, ok := params["pubkey"]
	if !ok {
		http.Error(w, "invalid pubkey", http.StatusBadRequest)
		return
	}

	if err := removeRequest.Verify(pubkey); err != nil {
		log.Printf("failed to verify webhook request: %v", err)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	h := sha256.New()
	h.Write([]byte(removeRequest.HookKey))

	err := s.store.Remove(context.Background(), pubkey, hex.EncodeToString(h.Sum(nil)))
	if err != nil {
		log.Printf(
			"failed to remove webhook for pubkey %v hookKey %v: %v",
			pubkey,
			removeRequest.HookKey,
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.WriteHeader(http.StatusOK)
}

/*
HandleRequest handles the request from the external caller and forward it to the node.
*/
func (l *WebhooksRouter) RequestHandler(requestType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		pubkey, ok := params["pubkey"]
		if !ok {
			http.Error(w, "invalid pubkey", http.StatusBadRequest)
			return
		}

		hookKeyHash, ok := params["hookKeyHash"]
		if !ok {
			http.Error(w, "invalid pubkey", http.StatusBadRequest)
			return
		}

		webhook, err := l.store.Get(context.Background(), pubkey, hookKeyHash)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if webhook == nil {
			http.Error(w, "webhook not found", http.StatusNotFound)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		bodyBytes, err := json.Marshal(body)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		request := InvokeWebhookRequest{
			Template: requestType,
			Data:     bodyBytes,
		}
		jsonBytes, err := json.Marshal(request)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		response, err := l.channel.SendRequest(webhook.Url, string(jsonBytes), w)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write([]byte(response))
	}
}
