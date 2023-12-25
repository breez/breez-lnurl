package webhook

import (
	"context"
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

type AddWebhookRequest struct {
	HookID    string `json:"hook_id"`
	Url       string `json:"url"`
	Signature string `json:"signature"`
}

type InvokeWebhookRequest struct {
	Template string          `json:"template"`
	Data     json.RawMessage `json:"data"`
}

func (w *AddWebhookRequest) Verify(pubkey string) error {
	verifiedPubkey, err := lightning.VerifyMessage([]byte(w.Url), w.Signature)
	if err != nil {
		return err
	}
	if pubkey != hex.EncodeToString(verifiedPubkey.SerializeCompressed()) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

type WebhooksRouter struct {
	store   persist.Store
	channel channel.WebhookChannel
}

func NewWebhookRouter(store persist.Store, channel channel.WebhookChannel) *WebhooksRouter {
	return &WebhooksRouter{
		store:   store,
		channel: channel,
	}
}

/*
Set adds a webhook for a given pubkey and a unique identifier.
The key enables the caller to replace existing hook without deleting it.
*/
func (s *WebhooksRouter) Set(w http.ResponseWriter, r *http.Request) {
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

	err := s.store.Set(context.Background(), persist.Webhook{
		Pubkey: pubkey,
		Url:    addRequest.Url,
		HookID: addRequest.HookID,
	})

	if err != nil {
		log.Printf(
			"failed to add webhook for %x for notifications on url %s: %v",
			pubkey,
			addRequest.Url,
			err,
		)

		w.WriteHeader(200)
	}
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

		hookKey, ok := params["hookKey"]
		if !ok {
			http.Error(w, "invalid pubkey", http.StatusBadRequest)
			return
		}

		webhook, err := l.store.Get(context.Background(), pubkey, hookKey)
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
