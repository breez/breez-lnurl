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

const (
	// https://datatracker.ietf.org/doc/html/rfc5322#section-3.4.1
	// https://stackoverflow.com/a/201378
	USERNAME_VALIDATION_REGEX = "^(?:[a-zA-Z0-9!#$%&'*+\\/=?^_`{|}~-]+(?:\\.[a-z0-9!#$%&'*+\\/=?^_`{|}~-]+)*|\"(?:[\x01-\x08\x0b\x0c\x0e-\x1f\x21\x23-\x5b\x5d-\x7f]|\\[\x01-\x09\x0b\x0c\x0e-\x7f])*\")$"
	// https://www.rfc-editor.org/errata/eid1690
	MAX_USERNAME_LENGTH = 64
)

type RegisterLnurlPayRequest struct {
	Username   *string `json:"username"`
	Time       int64   `json:"time"`
	WebhookUrl string  `json:"webhook_url"`
	Signature  string  `json:"signature"`
}

type RegisterRecoverLnurlPayResponse struct {
	Lnurl            string  `json:"lnurl"`
	LightningAddress *string `json:"lightning_address,omitempty"`
}

func (w *RegisterLnurlPayRequest) Verify(pubkey string) error {
	messageToVerify := fmt.Sprintf("%v-%v", w.Time, w.WebhookUrl)
	if w.Username != nil {
		username := *w.Username
		if len(username) > MAX_USERNAME_LENGTH {
			return fmt.Errorf("invalid username length %v", username)
		}
		if ok, err := regexp.MatchString(USERNAME_VALIDATION_REGEX, username); !ok || err != nil {
			return fmt.Errorf("invalid username %v", username)
		}
		messageToVerify = fmt.Sprintf("%v-%v", messageToVerify, username)
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
	body, err := marshalRegisterRecoverLnurlPayResponse(lnurlUri, webhook.Username, s.rootURL.Host)
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
	webhook, err := s.store.Set(r.Context(), persist.Webhook{
		Pubkey:   pubkey,
		Username: addRequest.Username,
		Url:      addRequest.WebhookUrl,
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

	log.Printf("registration added: pubkey:%v\n", pubkey)
	lnurlUri := fmt.Sprintf("%v/lnurlp/%v", s.rootURL, pubkey)
	body, err := marshalRegisterRecoverLnurlPayResponse(lnurlUri, webhook.Username, s.rootURL.Host)
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
func marshalRegisterRecoverLnurlPayResponse(lnurlUri string, username *string, host string) ([]byte, error) {
	lnurl, err := encodeLnurl(lnurlUri)
	if err != nil {
		return nil, err
	}
	var lightningAddress *string
	if username != nil {
		lnAddr := fmt.Sprintf("%v@%v", *username, host)
		lightningAddress = &lnAddr
	}
	return json.Marshal(RegisterRecoverLnurlPayResponse{
		Lnurl:            lnurl,
		LightningAddress: lightningAddress,
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
