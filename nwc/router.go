package nwc

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/breez/breez-lnurl/persist"
	nwc "github.com/breez/breez-lnurl/persist/nwc"
	"github.com/breez/lspd/lightning"
	"github.com/gorilla/mux"
)

type NostrEventsRouter struct {
	store   *persist.Store
	manager *NostrManager
	rootURL *url.URL
}

func RegisterNostrEventsRouter(router *mux.Router, rootURL *url.URL, store *persist.Store, cleanupService *nwc.CleanupService) {
	NostrEventsRouter := &NostrEventsRouter{
		store:   store,
		manager: NewNostrManager(store),
		rootURL: rootURL,
	}
	NostrEventsRouter.manager.Start()
	cleanupService.OnCleanup(NostrEventsRouter.manager.Resubscribe)
	router.HandleFunc("/nwc/{pubkey}", NostrEventsRouter.Register).Methods("POST")
	router.HandleFunc("/nwc/{pubkey}", NostrEventsRouter.Unregister).Methods("DELETE")
}

type RegisterNostrEventsRequest struct {
	WebhookUrl string   `json:"webhookUrl"`
	AppPubkey  string   `json:"appPubkey"`
	Relays     []string `json:"relays"`
	Signature  string   `json:"signature"`
}

func (w *RegisterNostrEventsRequest) Verify(pubkey string) error {
	messageToVerify := fmt.Sprintf("%v-%v-%v", w.WebhookUrl, w.AppPubkey, w.Relays)
	verifiedPubkey, err := lightning.VerifyMessage([]byte(messageToVerify), w.Signature)
	if err != nil {
		return err
	}
	if pubkey != hex.EncodeToString(verifiedPubkey.SerializeCompressed()) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

/*
Register adds a registration for a given pubkey, overwriting it if already present
*/
func (s *NostrEventsRouter) Register(w http.ResponseWriter, r *http.Request) {
	var registerRequest RegisterNostrEventsRequest
	if err := json.NewDecoder(r.Body).Decode(&registerRequest); err != nil {
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

	if err := registerRequest.Verify(pubkey); err != nil {
		log.Printf("failed to verify registration request: %v", err)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	err := s.store.Nwc.Set(r.Context(), nwc.Webhook{
		UserPubkey: pubkey,
		Url:        registerRequest.WebhookUrl,
		AppPubkey:  registerRequest.AppPubkey,
		Relays:     registerRequest.Relays,
	})
	if err != nil {
		log.Printf("failed to persist nwc details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := s.manager.Resubscribe(); err != nil {
		log.Printf("failed to resubscribe to Nostr events: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Printf("registration added: pubkey:%v\n", pubkey)
	w.Write([]byte("Pubkey registered successfully"))
}
