package webhook

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/breez/breez-lnurl/channel"
	"github.com/breez/breez-lnurl/persist"
	"github.com/breez/lspd/lightning"
	"github.com/gorilla/mux"
)

type AddWebhookRequest struct {
	Time      int64  `json:"time"`
	HookKey   string `json:"hook_key"`
	Url       string `json:"url"`
	Signature string `json:"signature"`
}

func (w *AddWebhookRequest) Verify(pubkey string) error {
	messageToVerify := fmt.Sprintf("%v-%v-%v", w.Time, w.HookKey, w.Url)
	verifiedPubkey, err := lightning.VerifyMessage([]byte(messageToVerify), w.Signature)
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
	messageToVerify := fmt.Sprintf("%v-%v", w.Time, w.HookKey)
	verifiedPubkey, err := lightning.VerifyMessage([]byte(messageToVerify), w.Signature)
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

func RegisterWebhookRouter(rootRouter *mux.Router, store persist.Store, channel channel.WebhookChannel) {
	webhookRouter := &WebhooksRouter{
		store:   store,
		channel: channel,
	}
	// Set webhook for a specific key
	rootRouter.HandleFunc("/webhooks/{pubkey}", webhookRouter.set).Methods("POST")
	// Delete webhook for a specific key
	rootRouter.HandleFunc("/webhooks/{pubkey}", webhookRouter.remove).Methods("DELETE")
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
	hash := hex.EncodeToString(h.Sum(nil))
	err := s.store.Set(r.Context(), persist.Webhook{
		Pubkey:      pubkey,
		Url:         addRequest.Url,
		HookKeyHash: hash,
	})

	if err != nil {
		log.Printf(
			"failed to add webhook for %x for notifications on url %s: %v",
			pubkey,
			addRequest.Url,
			err,
		)

		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	log.Printf("webhook added: pubkey:%v hash: %v\n", pubkey, hash)
	w.WriteHeader(http.StatusOK)
}

/*
Remove deletes a webhook for a given pubkey and a unique identifier.
*/
func (s *WebhooksRouter) remove(w http.ResponseWriter, r *http.Request) {
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

	err := s.store.Remove(r.Context(), pubkey, hex.EncodeToString(h.Sum(nil)))
	if err != nil {
		log.Printf(
			"failed to remove webhook for pubkey %v hookKey %v: %v",
			pubkey,
			removeRequest.HookKey,
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	log.Printf("webhook removed: pubkey:%v hash: %v\n", pubkey, removeRequest.HookKey)
	w.WriteHeader(http.StatusOK)
}
