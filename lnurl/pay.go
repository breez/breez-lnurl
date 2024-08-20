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
	"time"

	"log"

	"github.com/breez/breez-lnurl/channel"
	"github.com/breez/breez-lnurl/persist"
	"github.com/breez/lspd/lightning"
	"github.com/gorilla/mux"
)

type RegisterLnurlPayRequest struct {
	Username   *string `json:"username"`
	Time       int64   `json:"time"`
	WebhookUrl string  `json:"webhook_url"`
	Signature  string  `json:"signature"`
}

type RegisterLnurlPayResponse struct {
	Lnurl            string  `json:"lnurl"`
	LightningAddress *string `json:"lightning_address,omitempty"`
}

func (w *RegisterLnurlPayRequest) Verify(pubkey string) error {
	messageToVerify := fmt.Sprintf("%v-%v", w.Time, w.WebhookUrl)
	if w.Username != nil {
		if ok, err := regexp.MatchString("^\\w+$", *w.Username); !ok || err != nil {
			return fmt.Errorf("invalid username")
		}
		messageToVerify = fmt.Sprintf("%v-%v", messageToVerify, *w.Username)
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

type UnregisterLnurlPayRequest struct {
	Time       int64  `json:"time"`
	WebhookUrl string `json:"webhook_url"`
	Signature  string `json:"signature"`
}

func (w *UnregisterLnurlPayRequest) Verify(pubkey string) error {
	if math.Abs(float64(time.Now().Unix()-w.Time)) > 30 {
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
	channel channel.WebhookChannel
	rootURL *url.URL
}

func RegisterLnurlPayRouter(router *mux.Router, rootURL *url.URL, store persist.Store, channel channel.WebhookChannel) {
	lnurlPayRouter := &LnurlPayRouter{
		store:   store,
		channel: channel,
		rootURL: rootURL,
	}
	router.HandleFunc("/lnurlpay/{pubkey}", lnurlPayRouter.Register).Methods("POST")
	router.HandleFunc("/lnurlpay/{pubkey}", lnurlPayRouter.Unregister).Methods("DELETE")
	router.HandleFunc("/.well-known/lnurlp/{identifier}", lnurlPayRouter.HandleLnurlPay).Methods("GET")
	router.HandleFunc("/lnurlp/{identifier}", lnurlPayRouter.HandleLnurlPay).Methods("GET")
	router.HandleFunc("/lnurlpay/{identifier}/invoice", lnurlPayRouter.HandleInvoice).Methods("GET")
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
	webhook, err := s.store.Set(r.Context(), persist.Webhook{
		Pubkey:   pubkey,
		Username: addRequest.Username,
		Url:      addRequest.WebhookUrl,
	})

	if err != nil {
		log.Printf(
			"failed to register for %x for notifications on url %s: %v",
			pubkey,
			addRequest.WebhookUrl,
			err,
		)

		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Printf("registration added: pubkey:%v\n", pubkey)
	lnurl, err := encodeLnurl(fmt.Sprintf("%v/lnurlp/%v", s.rootURL, pubkey))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var lightningAddress *string
	if webhook.Username != nil {
		lnAddr := fmt.Sprintf("%v@%v", *webhook.Username, s.rootURL.Host)
		lightningAddress = &lnAddr
	}
	body, err := json.Marshal(RegisterLnurlPayResponse{
		Lnurl:            lnurl,
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
func (s *LnurlPayRouter) Unregister(w http.ResponseWriter, r *http.Request) {
	var removeRequest UnregisterLnurlPayRequest
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

	err := s.store.Remove(r.Context(), pubkey, removeRequest.WebhookUrl)
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

func writeJsonResponse(w http.ResponseWriter, response interface{}) {
	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Write(jsonBytes)
}
