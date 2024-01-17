package lnurl

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"log"

	"github.com/breez/breez-lnurl/channel"
	"github.com/breez/breez-lnurl/persist"
	"github.com/breez/lspd/lightning"
	"github.com/gorilla/mux"
)

type RegisterLnurlPayRequest struct {
	Time       int64  `json:"time"`
	HookKey    string `json:"hook_key"`
	WebhookUrl string `json:"webhook_url"`
	Signature  string `json:"signature"`
}

type RegisterLnurlPayResponse struct {
	Lnurl string `json:"lnurl"`
}

func (w *RegisterLnurlPayRequest) Verify(pubkey string) error {
	messageToVerify := fmt.Sprintf("%v-%v-%v", w.Time, w.HookKey, w.WebhookUrl)
	verifiedPubkey, err := lightning.VerifyMessage([]byte(messageToVerify), w.Signature)
	if err != nil {
		return err
	}
	if pubkey != hex.EncodeToString(verifiedPubkey.SerializeCompressed()) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

type UnegisterLnurlPayRequest struct {
	Time      int64  `json:"time"`
	HookKey   string `json:"hook_key"`
	Signature string `json:"signature"`
}

func (w *UnegisterLnurlPayRequest) Verify(pubkey string) error {
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
	router.HandleFunc("/lnurlpay/{pubkey}/{hookKeyHash}", lnurlPayRouter.HandleInfo).Methods("GET")
	router.HandleFunc("/lnurlpay/{pubkey}/{hookKeyHash}/invoice", lnurlPayRouter.HandleInvoice).Methods("GET")
}

/*
Set adds a regisration for a given pubkey and a unique identifier.
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
	h := sha256.New()
	h.Write([]byte(addRequest.HookKey))
	hash := hex.EncodeToString(h.Sum(nil))
	err := s.store.Set(r.Context(), persist.Webhook{
		Pubkey:      pubkey,
		Url:         addRequest.WebhookUrl,
		HookKeyHash: hash,
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

	log.Printf("registration added: pubkey:%v hash: %v\n", pubkey, hash)
	lnurl, err := encodeLnurl(fmt.Sprintf("%v/lnurlpay/%v/%v", s.rootURL, pubkey, hash))
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
Remove deletes a regisration for a given pubkey and a unique identifier.
*/
func (s *LnurlPayRouter) Unregister(w http.ResponseWriter, r *http.Request) {
	var removeRequest UnegisterLnurlPayRequest
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
	h := sha256.New()
	h.Write([]byte(removeRequest.HookKey))

	err := s.store.Remove(r.Context(), pubkey, hex.EncodeToString(h.Sum(nil)))
	if err != nil {
		log.Printf(
			"failed unregister for pubkey %v hookKey %v: %v",
			pubkey,
			removeRequest.HookKey,
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	log.Printf("registration removed: pubkey:%v hash: %v\n", pubkey, removeRequest.HookKey)
	w.WriteHeader(http.StatusOK)
}

/*
HandleInfo handles the initial request of lnurl pay protocol.
*/
func (l *LnurlPayRouter) HandleInfo(w http.ResponseWriter, r *http.Request) {
	pubkey, hookKeyHash, err := getParams(r)
	if err != nil {
		log.Printf("invalid params, err:%v", err)
		http.Error(w, "unexpected error", http.StatusInternalServerError)
		return
	}

	webhook, err := l.store.Get(r.Context(), pubkey, hookKeyHash)
	if err != nil {
		writeJsonResponse(w, NewLnurlPayErrorResponse("lnurl not found"))
		return
	}
	if webhook == nil {
		http.Error(w, "webhook not found", http.StatusNotFound)
		return
	}

	callbackURL := fmt.Sprintf("%v/lnurlpay/%v/%v/invoice", l.rootURL.String(), pubkey, hookKeyHash)
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
		log.Printf("failed to send request to webhook pubkey:%v, err:%v", pubkey, err)
		writeJsonResponse(w, NewLnurlPayErrorResponse("unavailable"))
		return
	}
	w.Write([]byte(response))
}

/*
HandleInvoice handles the seconds request of lnurl pay protocol.
*/
func (l *LnurlPayRouter) HandleInvoice(w http.ResponseWriter, r *http.Request) {
	pubkey, hookKeyHash, err := getParams(r)
	if err != nil {
		log.Printf("invalid params, err:%v", err)
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

	webhook, err := l.store.Get(r.Context(), pubkey, hookKeyHash)
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
		log.Printf("failed to send request to webhook pubkey:%v, err:%v", pubkey, err)
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

func getParams(r *http.Request) (string, string, error) {
	params := mux.Vars(r)
	pubkey, ok := params["pubkey"]
	if !ok {
		return "", "", errors.New("invalid pubkey")
	}

	hookKeyHash, ok := params["hookKeyHash"]
	if !ok {
		return "", "", errors.New("invalid hook key hash")
	}
	return pubkey, hookKeyHash, nil
}
