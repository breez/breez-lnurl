package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

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

func NewServer(internalURL *url.URL, externalURL *url.URL, storage persist.Store, proxyEndpoints []string) *Server {
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

func initRootHandler(externalURL *url.URL, storage persist.Store, proxyEndpoints []string) *mux.Router {
	rootRouter := mux.NewRouter()

	// start the cleanup service
	go func() {
		persist.NewCleanupService(storage).Start(context.Background())
	}()

	webhookChannel := channel.NewHttpCallbackChannel(fmt.Sprintf("%v/response", externalURL.String()))

	// Routes to manage webhooks.
	webhookRoutes := webhook.NewWebhookRouter(storage, webhookChannel)
	rootRouter.HandleFunc("/webhooks/{pubkey}", webhookRoutes.Set).Methods("POST")

	// Routes to handle external communication with the node using webhooks.
	for _, e := range proxyEndpoints {
		rootRouter.HandleFunc(fmt.Sprintf("/{pubkey}/{hookKey}/%v", e), webhookRoutes.RequestHandler(e)).Methods("GET")
	}

	rootRouter.HandleFunc("/response/{responseID}", webhookChannel.HandleResponse).Methods("POST")
	return rootRouter
}
