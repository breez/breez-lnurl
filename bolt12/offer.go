package bolt12

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"log"

	"github.com/breez/breez-lnurl/dns"
	"github.com/breez/breez-lnurl/persist"
	"github.com/breez/lspd/lightning"
	"github.com/gorilla/mux"
)

const (
	// https://datatracker.ietf.org/doc/html/rfc5322#section-3.4.1
	// https://stackoverflow.com/a/201378
	USERNAME_VALIDATION_REGEX = "^(?:[a-zA-Z0-9!#$%&'*+\\/=?^_`{|}~-]+(?:\\.[a-z0-9!#$%&'*+\\/=?^_`{|}~-]+)*|\"(?:[\x01-\x08\x0b\x0c\x0e-\x1f\x21\x23-\x5b\x5d-\x7f]|\\[\x01-\x09\x0b\x0c\x0e-\x7f])*\")$"
	// https://www.rfc-editor.org/errata/eid1690
	MAX_USERNAME_LENGTH = 64
)

type RegisterBolt12OfferRequest struct {
	Time      int64  `json:"time"`
	Username  string `json:"username"`
	Offer     string `json:"offer"`
	Signature string `json:"signature"`
}

type RegisterRecoverBolt12OfferResponse struct {
	LightningAddress string `json:"lightning_address"`
}

func (w *RegisterBolt12OfferRequest) Verify(pubkey string) error {
	if len(w.Username) > MAX_USERNAME_LENGTH {
		return fmt.Errorf("invalid username length %v", w.Username)
	}
	if ok, err := regexp.MatchString(USERNAME_VALIDATION_REGEX, w.Username); !ok || err != nil {
		return fmt.Errorf("invalid username %v", w.Username)
	}

	messageToVerify := fmt.Sprintf("%v-%v-%v", w.Time, w.Username, w.Offer)
	verifiedPubkey, err := lightning.VerifyMessage([]byte(messageToVerify), w.Signature)
	if err != nil {
		return err
	}
	if pubkey != hex.EncodeToString(verifiedPubkey.SerializeCompressed()) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

type UnregisterRecoverBolt12OfferRequest struct {
	Time      int64  `json:"time"`
	Offer     string `json:"offer"`
	Signature string `json:"signature"`
}

func (w *UnregisterRecoverBolt12OfferRequest) Verify(pubkey string) error {
	if math.Abs(float64(time.Now().Unix()-w.Time)) > 30 {
		return errors.New("invalid time")
	}
	messageToVerify := fmt.Sprintf("%v-%v", w.Time, w.Offer)
	verifiedPubkey, err := lightning.VerifyMessage([]byte(messageToVerify), w.Signature)
	if err != nil {
		return err
	}
	if pubkey != hex.EncodeToString(verifiedPubkey.SerializeCompressed()) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

type Bolt12OfferRouter struct {
	store   persist.Store
	dns     dns.DnsService
	rootURL *url.URL
}

func RegisterBolt12OfferRouter(router *mux.Router, rootURL *url.URL, store persist.Store, dns dns.DnsService) {
	Bolt12OfferRouter := &Bolt12OfferRouter{
		store:   store,
		dns:     dns,
		rootURL: rootURL,
	}
	router.HandleFunc("/bolt12offer/{pubkey}", Bolt12OfferRouter.Register).Methods("POST")
	router.HandleFunc("/bolt12offer/{pubkey}", Bolt12OfferRouter.Unregister).Methods("DELETE")
	router.HandleFunc("/bolt12offer/{pubkey}/recover", Bolt12OfferRouter.Recover).Methods("POST")
}

/*
Recover retreives the registered lightning address for a given pubkey.
*/
func (s *Bolt12OfferRouter) Recover(w http.ResponseWriter, r *http.Request) {
	var recoverRequest UnregisterRecoverBolt12OfferRequest
	if err := json.NewDecoder(r.Body).Decode(&recoverRequest); err != nil {
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

	if err := recoverRequest.Verify(pubkey); err != nil {
		log.Printf("failed to verify recover request: %v", err)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	lastPkUsername, err := s.store.GetPubkeyDetails(r.Context(), pubkey)
	if err != nil || lastPkUsername == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	lightningAddress := fmt.Sprintf("%v@%v", lastPkUsername.Username, s.rootURL.Host)
	body, err := json.Marshal(RegisterRecoverBolt12OfferResponse{
		LightningAddress: lightningAddress,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(body)
}

/*
Register adds a registration for a given pubkey and a unique identifier.
The key enables the caller to replace existing offer without deleting it.
*/
func (s *Bolt12OfferRouter) Register(w http.ResponseWriter, r *http.Request) {
	var addRequest RegisterBolt12OfferRequest
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
		log.Printf("failed to verify registration request: %v", err)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	// Get the last pubkey username for the pubkey to use it to check if the offer has changed
	lastPkUsername, _ := s.store.GetPubkeyDetails(r.Context(), pubkey)
	updatedPkUsername, err := s.store.SetPubkeyDetails(r.Context(), pubkey, addRequest.Username, &addRequest.Offer)

	if err != nil {
		if serr, ok := err.(*persist.ErrorUsernameConflict); ok {
			http.Error(w, serr.Error(), http.StatusConflict)
			return
		}
		log.Printf(
			"failed to register for %x for notifications on offer %s: %v",
			pubkey,
			addRequest.Offer,
			err,
		)

		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Update the BIP353 DNS TXT records
	if updatedPkUsername.Offer != nil {
		shouldSetOffer := lastPkUsername == nil || lastPkUsername.Offer == nil
		username := updatedPkUsername.Username
		offer := *updatedPkUsername.Offer

		if lastPkUsername != nil && lastPkUsername.Offer != nil {
			// If the last webhook exists, we need to check if the username or offer has changed
			lastUsername := lastPkUsername.Username
			lastOffer := *lastPkUsername.Offer
			shouldSetOffer = username != lastUsername || offer != lastOffer

			if username != lastUsername {
				if err = s.dns.Remove(lastUsername); err != nil {
					log.Printf("failed to remove DNS TXT record for %v: %v", lastUsername, err)
				}
			}
		}

		if shouldSetOffer {
			ttl, err := s.dns.Set(username, offer)
			if err != nil {
				log.Printf("failed to set DNS TXT record for %v, %v: %v", username, offer, err)
			}
			// Only set the offer if the DNS service returns a TTL
			maybeOffer := &offer
			if ttl == 0 {
				maybeOffer = nil
			}
			s.store.SetPubkeyDetails(r.Context(), pubkey, username, maybeOffer)
		}
	}

	log.Printf("registration added: pubkey:%v\n", pubkey)
	lightningAddress := fmt.Sprintf("%v@%v", updatedPkUsername.Username, s.rootURL.Host)
	body, err := json.Marshal(RegisterRecoverBolt12OfferResponse{
		LightningAddress: lightningAddress,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(body)
}

/*
Unregister deletes a registration for a given pubkey and a unique identifier.
*/
func (s *Bolt12OfferRouter) Unregister(w http.ResponseWriter, r *http.Request) {
	var removeRequest UnregisterRecoverBolt12OfferRequest
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
		log.Printf("failed to verify request: %v", err)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	// Return 200 if the pubkey username is not found
	pkUsername, err := s.store.GetPubkeyDetails(r.Context(), pubkey)
	if err != nil || pkUsername == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Remove the DNS TXT record for this username/offer
	if pkUsername.Offer != nil {
		username := pkUsername.Username
		if err = s.dns.Remove(username); err != nil {
			log.Printf("failed to remove DNS TXT record for %v: %v", username, err)
		}
		s.store.SetPubkeyDetails(r.Context(), pubkey, username, nil)
	}

	log.Printf("registration removed: pubkey:%v offer: %v\n", pubkey, removeRequest.Offer)
	w.WriteHeader(http.StatusOK)
}
