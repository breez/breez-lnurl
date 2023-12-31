package channel

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

const (
	callbackTimeout = 30 * time.Second
)

type WebhookChannel interface {
	SendRequest(url string, payload string, rw http.ResponseWriter) (string, error)
}

type WebhookChannelRequestPayload struct {
	Template string `json:"webhook_callback_message"`
	Data     struct {
		CallbackURL    string `json:"callback_url"`
		MessagePayload string `json:"message_payload"`
	} `json:"data"`
}

type PendingRequest struct {
	writer http.ResponseWriter
	result chan string
}

type HttpCallbackChannel struct {
	sync.Mutex
	callbackBaseURL string
	random          *rand.Rand
	pendingRequests map[uint64]*PendingRequest
}

func NewHttpCallbackChannel(router *mux.Router, callbackBaseURL string) *HttpCallbackChannel {

	channel := &HttpCallbackChannel{
		callbackBaseURL: callbackBaseURL,
		random:          rand.New(rand.NewSource(time.Now().UnixNano())),
		pendingRequests: make(map[uint64]*PendingRequest),
	}

	// We register the route for node responses via the callback route
	router.HandleFunc("/response/{responseID}", channel.HandleResponse).Methods("POST")

	return channel
}

func (p *HttpCallbackChannel) SendRequest(url string, payload string, rw http.ResponseWriter) (string, error) {
	reqID := p.random.Uint64()
	callbackURL := fmt.Sprintf("%s/%d", p.callbackBaseURL, reqID)
	webhookPayload := WebhookChannelRequestPayload{Template: "webhook_callback_message", Data: struct {
		CallbackURL    string "json:\"callback_url\""
		MessagePayload string "json:\"message_payload\""
	}{CallbackURL: callbackURL, MessagePayload: payload}}

	jsonBytes, err := json.Marshal(webhookPayload)
	if err != nil {
		return "", err
	}
	pendingRequest := &PendingRequest{
		writer: rw,
		result: make(chan string, 1),
	}
	p.Lock()
	p.pendingRequests[reqID] = pendingRequest
	p.Unlock()

	defer func() {
		p.Lock()
		pendingRequest, ok := p.pendingRequests[reqID]
		if ok {
			close(pendingRequest.result)
			delete(p.pendingRequests, reqID)
		}
		p.Unlock()
	}()

	httpRes, err := http.Post(url, "application/json", strings.NewReader(string(jsonBytes)))
	if err != nil {
		return "", err
	}
	if httpRes.StatusCode != 200 {
		return "", errors.New("webhook proxy returned non-200 status code")
	}
	select {
	case result := <-pendingRequest.result:
		return result, nil
	case <-time.After(callbackTimeout):
		p.Lock()
		delete(p.pendingRequests, reqID)
		p.Unlock()
		return "", errors.New("timeout")
	}
}

func (p *HttpCallbackChannel) OnResponse(reqID uint64, payload string) error {
	p.Lock()
	pendingRequest, ok := p.pendingRequests[reqID]
	p.Unlock()

	if !ok {
		return errors.New("unknown request id")
	}
	pendingRequest.result <- payload
	return nil
}

/*
HandleResponse handles the response from the node.
*/
func (l *HttpCallbackChannel) HandleResponse(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	responseID, ok := params["responseID"]
	if !ok {
		http.Error(w, "invalid pubkey", http.StatusBadRequest)
		return
	}
	reqID, err := strconv.ParseUint(responseID, 10, 64)
	if err != nil {
		http.Error(w, "invalid response", http.StatusBadRequest)
		return
	}
	all, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := l.OnResponse(reqID, string(all)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
