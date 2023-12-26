package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/breez/breez-lnurl/channel"
	"github.com/breez/breez-lnurl/persist"
	"github.com/breez/breez-lnurl/webhook"
	"github.com/gorilla/mux"
)

type Server struct {
	internalURL *url.URL
	externalURL *url.URL
	storage     persist.Store
	rootHandler *mux.Router
}

func NewServer(internalURL *url.URL, externalURL *url.URL, storage persist.Store, proxyEndpoints map[string][]string) *Server {
	server := &Server{
		internalURL: internalURL,
		externalURL: externalURL,
		storage:     storage,
		rootHandler: initRootHandler(externalURL, storage, proxyEndpoints),
	}

	return server
}

func (s *Server) Serve() error {
	return http.ListenAndServe(s.internalURL.Host, s.rootHandler)
}

func initRootHandler(externalURL *url.URL, storage persist.Store, proxyEndpoints map[string][]string) *mux.Router {
	rootRouter := mux.NewRouter()

	// start the cleanup service
	go func() {
		persist.NewCleanupService(storage).Start(context.Background())
	}()

	// The channel that handles the request/response cycle from the node.
	// This specific channel handles that by invoking the registered webhook to reach the node
	// providing a callback URL to the node.
	webhookChannel := channel.NewHttpCallbackChannel(rootRouter, fmt.Sprintf("%v/response", externalURL.String()))

	// Routes to manage webhooks.
	webhookRoutes := webhook.NewWebhookRouter(rootRouter, storage, webhookChannel)

	// Routes to handle external communication with the node using webhooks.
	// The endpoint name is part of the payload that is sent to the node which makes this
	// mechanism pretty generic.
	for feature, endpoints := range proxyEndpoints {
		for _, endpoint := range endpoints {
			u := fmt.Sprintf("/%v/{pubkey}/{hookKeyHash}/%v", feature, endpoint)
			rootRouter.HandleFunc(u, webhookRoutes.RequestHandler(strings.Join([]string{feature, endpoint}, "-"))).Methods("GET", "POST")
		}
	}

	return rootRouter
}
