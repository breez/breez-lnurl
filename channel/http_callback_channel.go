package channel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

const (
	CALLBACK_TIMEOUT = 30 * time.Second
)

type WebhookMessage struct {
	Template string                 `json:"template"`
	Data     map[string]interface{} `json:"data"`
}

type CallbackResponse struct {
	Body   []byte
	MaxAge *int64
}

type WebhookChannel interface {
	SendRequest(context context.Context, url string, message WebhookMessage, rw http.ResponseWriter) (*CallbackResponse, error)
}

type PendingRequest struct {
	id       uint64
	response chan CallbackResponse
}

type HttpCallbackChannel struct {
	sync.Mutex
	httpClient      *http.Client
	callbackBaseURL string
	random          *rand.Rand
	pendingRequests map[uint64]*PendingRequest
}

func NewHttpCallbackChannel(router *mux.Router, callbackBaseURL string) *HttpCallbackChannel {

	channel := &HttpCallbackChannel{
		httpClient:      http.DefaultClient,
		callbackBaseURL: callbackBaseURL,
		random:          rand.New(rand.NewSource(time.Now().UnixNano())),
		pendingRequests: make(map[uint64]*PendingRequest),
	}

	// We register the route for node responses via the callback route
	router.HandleFunc("/response/{responseID}", channel.HandleResponse).Methods("POST")

	return channel
}

func (p *HttpCallbackChannel) SendRequest(c context.Context, url string, message WebhookMessage, rw http.ResponseWriter) (*CallbackResponse, error) {
	reqID := p.random.Uint64()
	callbackURL := fmt.Sprintf("%s/%d", p.callbackBaseURL, reqID)
	message.Data["reply_url"] = callbackURL
	jsonBytes, err := json.Marshal(message)
	if err != nil {
		return nil, err
	}
	pendingRequest := &PendingRequest{
		id:       reqID,
		response: make(chan CallbackResponse, 1),
	}
	p.Lock()
	p.pendingRequests[reqID] = pendingRequest
	p.Unlock()

	// We only delete the request from the map and close the channel only if it was not deleted before.
	defer func() {
		p.Lock()
		req, ok := p.pendingRequests[reqID]
		if ok {
			p.deleteRequestAndClose(req)
		}
		p.Unlock()
	}()

	req, err := http.NewRequestWithContext(c, "POST", url, strings.NewReader(string(jsonBytes)))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")

	log.Printf("Sending webhook callback message %v", string(jsonBytes))
	httpRes, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if httpRes.StatusCode != 200 {
		return nil, errors.New("webhook proxy returned non-200 status code")
	}
	select {
	case response := <-pendingRequest.response:
		return &response, nil
	case <-c.Done():
		return nil, errors.New("canceled")
	case <-time.After(CALLBACK_TIMEOUT):
		return nil, errors.New("timeout")
	}
}

func (p *HttpCallbackChannel) OnResponse(reqID uint64, response CallbackResponse) error {
	p.Lock()
	defer p.Unlock()
	pendingRequest, ok := p.pendingRequests[reqID]
	if !ok {
		return errors.New("unknown request id")
	}
	pendingRequest.response <- response
	// We only delete the request from the map and close the channel.
	p.deleteRequestAndClose(pendingRequest)
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
	response := CallbackResponse{
		Body: all,
		MaxAge: getCacheControlMaxAge(r.Header),
	}
	if err := l.OnResponse(reqID, response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getCacheControlMaxAge(header http.Header) *int64 {
	cacheControl := header.Get("Cache-Control")
	if cacheControl == "" {
		return nil
	}
	for _, directive := range strings.Split(cacheControl, ",") {
		directive = strings.TrimSpace(directive)
		if strings.HasPrefix(directive, "max-age=") {
			maxAgeStr := strings.TrimPrefix(directive, "max-age=")
			maxAge, err := strconv.ParseInt(maxAgeStr, 10, 64)
			if err != nil {
				return nil
			}
			return &maxAge
		}
	}
	return nil
}

func (p *HttpCallbackChannel) deleteRequestAndClose(req *PendingRequest) {
	delete(p.pendingRequests, req.id)
	close(req.response)
}
