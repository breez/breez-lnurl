package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/breez/breez-lnurl/bolt12"
	"github.com/breez/breez-lnurl/cache"
	"github.com/breez/breez-lnurl/channel"
	"github.com/breez/breez-lnurl/dns"
	"github.com/breez/breez-lnurl/lnurl"
	"github.com/breez/breez-lnurl/nwc"
	"github.com/breez/breez-lnurl/persist"
	"github.com/gorilla/mux"
)

type Server struct {
	internalURL *url.URL
	externalURL *url.URL
	storage     *persist.Store
	dns         dns.DnsService
	cache       cache.CacheService
	rootHandler *mux.Router
}

func NewServer(internalURL *url.URL, externalURL *url.URL, storage *persist.Store, dns dns.DnsService, cache cache.CacheService) *Server {
	server := &Server{
		internalURL: internalURL,
		externalURL: externalURL,
		storage:     storage,
		dns:         dns,
		cache:       cache,
		rootHandler: initRootHandler(externalURL, storage, dns, cache),
	}

	return server
}

func (s *Server) Serve() error {
	return http.ListenAndServe("0.0.0.0:8080", s.rootHandler)
}

func initRootHandler(externalURL *url.URL, storage *persist.Store, dns dns.DnsService, cache cache.CacheService) *mux.Router {
	rootRouter := mux.NewRouter()

	// start the cleanup service
	cleanup := persist.NewCleanupService(storage)
	cleanup.Start(context.Background())

	// The channel that handles the request/response cycle from the node.
	// This specific channel handles that by invoking the registered webhook to reach the node
	// providing a callback URL to the node.
	webhookChannel := channel.NewHttpCallbackChannel(rootRouter, fmt.Sprintf("%v/response", externalURL.String()))

	// Routes to handle lnurl pay protocol.
	lnurl.RegisterLnurlPayRouter(rootRouter, externalURL, storage, dns, cache, webhookChannel)

	// Routes to handle BOLT12 Offers.
	bolt12.RegisterBolt12OfferRouter(rootRouter, externalURL, storage, dns)

	// Routes to handle Nostr event subscriptions
	nwc.RegisterNostrEventsRouter(rootRouter, externalURL, storage, cleanup.Nwc)

	return rootRouter
}
