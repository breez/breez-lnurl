package lnurl

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"log"

	"github.com/breez/breez-lnurl/channel"
	"github.com/breez/breez-lnurl/persist"
	"github.com/gorilla/mux"
)

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
	router.HandleFunc("/lnurlpay/{pubkey}/{hookKeyHash}", lnurlPayRouter.HandleInfo).Methods("GET")
	router.HandleFunc("/lnurlpay/{pubkey}/{hookKeyHash}/invoice", lnurlPayRouter.HandleInvoice).Methods("GET")
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
	request := LnurlPayWebhookPayload{
		Template: "lnurlpay_info",
		Data: map[string]interface{}{
			"lnurlpay_info": map[string]interface{}{
				"callback_url": callbackURL,
			},
		},
	}
	jsonBytes, err := json.Marshal(request)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	response, err := l.channel.SendRequest(r.Context(), webhook.Url, "lnurlpay_info", string(jsonBytes), w)
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

	request := LnurlPayWebhookPayload{
		Template: "lnurlpay_invoice",
		Data: map[string]interface{}{
			"lnurlpay_invoice": map[string]interface{}{
				"amount": amountNum,
			},
		},
	}
	jsonBytes, err := json.Marshal(request)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	response, err := l.channel.SendRequest(r.Context(), webhook.Url, "lnurlpay_invoice", string(jsonBytes), w)
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
