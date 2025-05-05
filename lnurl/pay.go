package lnurl

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"log"

	"github.com/breez/breez-lnurl/channel"
	"github.com/breez/breez-lnurl/constant"
	"github.com/breez/breez-lnurl/dns"
	"github.com/breez/breez-lnurl/persist"
	"github.com/breez/lspd/lightning"
	"github.com/gorilla/mux"
)

type RegisterLnurlPayRequest struct {
	Time       int64   `json:"time"`
	WebhookUrl string  `json:"webhook_url"`
	Username   *string `json:"username"`
	Offer      *string `json:"offer"`
	Signature  string  `json:"signature"`
}

type RegisterRecoverLnurlPayResponse struct {
	Lnurl            string  `json:"lnurl"`
	LightningAddress *string `json:"lightning_address,omitempty"`
	BIP353Address    *string `json:"bip353_address,omitempty"`
}

func (w *RegisterLnurlPayRequest) Verify(pubkey string) error {
	if math.Abs(float64(time.Now().Unix()-w.Time)) > constant.ACCEPTABLE_TIME_DIFF {
		return errors.New("invalid time")
	}
	messageToVerify := fmt.Sprintf("%v-%v", w.Time, w.WebhookUrl)
	if w.Username != nil {
		// Validate with username if present
		username := *w.Username
		if len(username) > constant.MAX_USERNAME_LENGTH {
			return fmt.Errorf("invalid username length %v", username)
		}
		if ok, err := regexp.MatchString(constant.USERNAME_VALIDATION_REGEX, username); !ok || err != nil {
			return fmt.Errorf("invalid username %v", username)
		}
		messageToVerify = fmt.Sprintf("%v-%v", messageToVerify, username)
		// Validate with offer if present
		if w.Offer != nil {
			offer := *w.Offer
			if !strings.HasPrefix(offer, "lno") {
				return fmt.Errorf("invalid offer %v", offer)
			}
			messageToVerify = fmt.Sprintf("%v-%v", messageToVerify, offer)
		}
	}
	verifiedPubkey, err := lightning.VerifyMessage([]byte(messageToVerify), w.Signature)
	if err != nil {
		return err
	}
	if pubkey != hex.EncodeToString(verifiedPubkey.SerializeCompressed()) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

type UnregisterRecoverLnurlPayRequest struct {
	Time       int64  `json:"time"`
	WebhookUrl string `json:"webhook_url"`
	Signature  string `json:"signature"`
}

func (w *UnregisterRecoverLnurlPayRequest) Verify(pubkey string) error {
	if math.Abs(float64(time.Now().Unix()-w.Time)) > constant.ACCEPTABLE_TIME_DIFF {
		return errors.New("invalid time")
	}
	messageToVerify := fmt.Sprintf("%v-%v", w.Time, w.WebhookUrl)
	verifiedPubkey, err := lightning.VerifyMessage([]byte(messageToVerify), w.Signature)
	if err != nil {
		return err
	}
	if pubkey != hex.EncodeToString(verifiedPubkey.SerializeCompressed()) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

type LnurlPayStatus struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type LnurlPayWebhookPayload struct {
	Template string                 `json:"template"`
	Data     map[string]interface{} `json:"data"`
}

func NewLnurlPayErrorResponse(reason string) LnurlPayStatus {
	return LnurlPayStatus{
		Status: "ERROR",
		Reason: reason,
	}
}

func NewLnurlPayOkResponse(reason string) LnurlPayStatus {
	return LnurlPayStatus{
		Status: "OK",
	}
}

type LnurlPayRouter struct {
	store   persist.Store
	dns     dns.DnsService
	channel channel.WebhookChannel
	rootURL *url.URL
}

func RegisterLnurlPayRouter(router *mux.Router, rootURL *url.URL, store persist.Store, dns dns.DnsService, channel channel.WebhookChannel) {
	lnurlPayRouter := &LnurlPayRouter{
		store:   store,
		dns:     dns,
		channel: channel,
		rootURL: rootURL,
	}
	router.HandleFunc("/lnurlpay/{pubkey}", lnurlPayRouter.Register).Methods("POST")
	router.HandleFunc("/lnurlpay/{pubkey}", lnurlPayRouter.Unregister).Methods("DELETE")
	router.HandleFunc("/lnurlpay/{pubkey}/recover", lnurlPayRouter.Recover).Methods("POST")
	router.HandleFunc("/.well-known/lnurlp/{identifier}", lnurlPayRouter.HandleLnurlPay).Methods("GET")
	router.HandleFunc("/lnurlp/{identifier}", lnurlPayRouter.HandleLnurlPay).Methods("GET")
	router.HandleFunc("/lnurlpay/{identifier}/invoice", lnurlPayRouter.HandleInvoice).Methods("GET")
}

/*
Recover retreives the registered LNURL/lightning address for a given pubkey.
*/
func (s *LnurlPayRouter) Recover(w http.ResponseWriter, r *http.Request) {
	var recoverRequest UnregisterRecoverLnurlPayRequest
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

	webhook, err := s.store.GetLastUpdated(r.Context(), pubkey)
	if err != nil || webhook == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	lnurlUri := fmt.Sprintf("%v/lnurlp/%v", s.rootURL, pubkey)
	body, err := marshalRegisterRecoverLnurlPayResponse(lnurlUri, webhook.Username, webhook.Offer, s.rootURL.Host)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(body)
}

/*
Register adds a registration for a given pubkey and a unique identifier.
The key enables the caller to replace existing hook without deleting it.
*/
func (s *LnurlPayRouter) Register(w http.ResponseWriter, r *http.Request) {
	var addRequest RegisterLnurlPayRequest
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

	// Get the last updated webhook for the pubkey to use it to check if the offer has changed
	lastWebhook, _ := s.store.GetLastUpdated(r.Context(), pubkey)
	updatedWebhook, err := s.store.Set(r.Context(), persist.Webhook{
		Pubkey:   pubkey,
		Url:      addRequest.WebhookUrl,
		Username: addRequest.Username,
	})

	if err != nil {
		if serr, ok := err.(*persist.ErrorUsernameConflict); ok {
			http.Error(w, serr.Error(), http.StatusConflict)
			return
		}
		log.Printf(
			"failed to register for %x for notifications on url %s: %v",
			pubkey,
			addRequest.WebhookUrl,
			err,
		)

		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Update the BIP353 DNS TXT records
	if addRequest.Username != nil && addRequest.Offer != nil {
		shouldSetOffer := lastWebhook == nil || lastWebhook.Offer == nil
		username := *addRequest.Username
		offer := *addRequest.Offer

		if lastWebhook != nil && lastWebhook.Username != nil && lastWebhook.Offer != nil {
			// If the last webhook exists, we need to check if the username or offer has changed
			lastUsername := *lastWebhook.Username
			lastOffer := *lastWebhook.Offer
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
			if ttl != 0 {
				// Only set the offer if the DNS service returns a TTL
				s.store.SetPubkeyDetails(r.Context(), pubkey, username, &offer)
			}
		}
	}

	log.Printf("registration added: pubkey:%v\n", pubkey)
	lnurlUri := fmt.Sprintf("%v/lnurlp/%v", s.rootURL, pubkey)
	body, err := marshalRegisterRecoverLnurlPayResponse(lnurlUri, updatedWebhook.Username, updatedWebhook.Offer, s.rootURL.Host)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(body)
}

/*
Unregister deletes a registration for a given pubkey and a unique identifier.
*/
func (s *LnurlPayRouter) Unregister(w http.ResponseWriter, r *http.Request) {
	var removeRequest UnregisterRecoverLnurlPayRequest
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

	// Return 200 if the webhook is not found
	webhook, err := s.store.GetLastUpdated(r.Context(), pubkey)
	if err != nil || webhook == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Remove the webhook from the store for the given pubkey
	err = s.store.Remove(r.Context(), pubkey, removeRequest.WebhookUrl)
	if err != nil {
		log.Printf(
			"failed unregister for pubkey %v url %v: %v",
			pubkey,
			removeRequest.WebhookUrl,
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Remove the DNS TXT record for this username/offer
	if webhook.Username != nil {
		username := *webhook.Username
		if err = s.dns.Remove(username); err != nil {
			log.Printf("failed to remove DNS TXT record for %v: %v", username, err)
		}
		s.store.SetPubkeyDetails(r.Context(), pubkey, username, nil)
	}

	log.Printf("registration removed: pubkey:%v url: %v\n", pubkey, removeRequest.WebhookUrl)
	w.WriteHeader(http.StatusOK)
}

/*
HandleLnurlPay handles the initial request of lnurl pay protocol.
*/
func (l *LnurlPayRouter) HandleLnurlPay(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	identifier, ok := params["identifier"]
	if !ok {
		log.Println("invalid params, err")
		http.Error(w, "unexpected error", http.StatusInternalServerError)
		return
	}

	webhook, err := l.store.GetLastUpdated(r.Context(), identifier)
	if err != nil {
		writeJsonResponse(w, NewLnurlPayErrorResponse("lnurl not found"))
		return
	}
	if webhook == nil {
		http.Error(w, "webhook not found", http.StatusNotFound)
		return
	}

	callbackURL := fmt.Sprintf("%v/lnurlpay/%v/invoice", l.rootURL.String(), identifier)
	message := channel.WebhookMessage{
		Template: "lnurlpay_info",
		Data: map[string]interface{}{
			"callback_url": callbackURL,
		},
	}

	response, err := l.channel.SendRequest(r.Context(), webhook.Url, message, w)
	if r.Context().Err() != nil {
		return
	}
	if err != nil {
		log.Printf("failed to send request to webhook pubkey:%v, err:%v", webhook.Pubkey, err)
		writeJsonResponse(w, NewLnurlPayErrorResponse("unavailable"))
		return
	}
	w.Write([]byte(response))
}

/*
HandleInvoice handles the seconds request of lnurl pay protocol.
*/
func (l *LnurlPayRouter) HandleInvoice(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	identifier, ok := params["identifier"]
	if !ok {
		log.Println("invalid params, err")
		http.Error(w, "unexpected error", http.StatusInternalServerError)
		return
	}

	amount := r.URL.Query().Get("amount")
	if amount == "" {
		writeJsonResponse(w, NewLnurlPayErrorResponse("missing amount"))
		return
	}
	amountNum, err := strconv.ParseUint(amount, 10, 64)
	if err != nil || amountNum == 0 {
		writeJsonResponse(w, NewLnurlPayErrorResponse("invalid amount"))
		return
	}

	webhook, err := l.store.GetLastUpdated(r.Context(), identifier)
	if err != nil {
		writeJsonResponse(w, NewLnurlPayErrorResponse("lnurl not found"))
		return
	}
	if webhook == nil {
		http.Error(w, "webhook not found", http.StatusNotFound)
		return
	}

	message := channel.WebhookMessage{
		Template: "lnurlpay_invoice",
		Data: map[string]interface{}{
			"amount": amountNum,
		},
	}
	response, err := l.channel.SendRequest(r.Context(), webhook.Url, message, w)
	if r.Context().Err() != nil {
		return
	}
	if err != nil {
		log.Printf("failed to send request to webhook pubkey:%v, err:%v", webhook.Pubkey, err)
		writeJsonResponse(w, NewLnurlPayErrorResponse("unavailable"))
		return
	}
	w.Write([]byte(response))
}

/* helper methods */
func marshalRegisterRecoverLnurlPayResponse(lnurlUri string, username *string, offer *string, host string) ([]byte, error) {
	lnurl, err := encodeLnurl(lnurlUri)
	if err != nil {
		return nil, err
	}
	var lightningAddress, bip353Address *string
	if username != nil {
		lnAddr := fmt.Sprintf("%v@%v", *username, host)
		lightningAddress = &lnAddr
		if offer != nil {
			bip353Address = &lnAddr
		}
	}
	return json.Marshal(RegisterRecoverLnurlPayResponse{
		Lnurl:            lnurl,
		LightningAddress: lightningAddress,
		BIP353Address:    bip353Address,
	})
}

func writeJsonResponse(w http.ResponseWriter, response interface{}) {
	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Write(jsonBytes)
}
