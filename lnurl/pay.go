package lnurl

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"log"

	"github.com/breez/breez-lnurl/channel"
	"github.com/breez/breez-lnurl/persist"
	"github.com/breez/lspd/lightning"
	"github.com/gorilla/mux"
)

type RegisterLnurlPayRequest struct {
	Time       int64  `json:"time"`
	WebhookUrl string `json:"webhook_url"`
	Signature  string `json:"signature"`
}

type RegisterLnurlPayResponse struct {
	Lnurl string `json:"lnurl"`
}

func (w *RegisterLnurlPayRequest) Verify(pubkey string) error {
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
	router.HandleFunc("/.well-known/lnurlp/{lightningAddressUser}", lnurlPayRouter.HandleLightningAddress).Methods("GET")
	router.HandleFunc("/lnurlpay/{lightningAddressUser}/invoice", lnurlPayRouter.HandleInvoice).Methods("GET")
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
	err := s.store.Set(r.Context(), persist.Webhook{
		Pubkey: pubkey,
		Url:    addRequest.WebhookUrl,
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
	lnurl, err := encodeLnurl(fmt.Sprintf("%v/.well-known/lnurlp/%v", s.rootURL, pubkey))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	body, err := json.Marshal(RegisterLnurlPayResponse{
		Lnurl: lnurl,
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
HandleLightningAddress handles the initial request of lnurl pay protocol.
*/
func (l *LnurlPayRouter) HandleLightningAddress(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	lightningAddressUser, ok := params["lightningAddressUser"]
	if !ok {
		log.Println("invalid params, err")
		http.Error(w, "unexpected error", http.StatusInternalServerError)
		return
	}

	webhook, err := l.store.GetLastUpdated(r.Context(), lightningAddressUser)
	if err != nil {
		writeJsonResponse(w, NewLnurlPayErrorResponse("lnurl not found"))
		return
	}
	if webhook == nil {
		http.Error(w, "webhook not found", http.StatusNotFound)
		return
	}

	callbackURL := fmt.Sprintf("%v/lnurlpay/%v/invoice", l.rootURL.String(), webhook.Pubkey)
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
	lightningAddressUser, ok := params["lightningAddressUser"]
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

	webhook, err := l.store.GetLastUpdated(r.Context(), lightningAddressUser)
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
