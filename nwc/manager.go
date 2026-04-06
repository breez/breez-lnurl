package nwc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"slices"

	"net/http"
	"strings"
	"sync"

	"fiatjaf.com/nostr"
	"github.com/breez/breez-lnurl/channel"
	"github.com/breez/breez-lnurl/persist"
	nwc "github.com/breez/breez-lnurl/persist/nwc"
)

type Subscription struct {
	ctx          context.Context
	cancel       context.CancelFunc
	eventChannel chan nostr.RelayEvent
	details      *nwc.SubscriptionDetails
}

type NostrManager struct {
	pool      *nostr.Pool
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.RWMutex
	isRunning bool
	subs      map[string]*Subscription
	store     *persist.Store
}

func NewNostrManager(store *persist.Store) *NostrManager {
	return &NostrManager{
		isRunning: false,
		store:     store,
		subs:      make(map[string]*Subscription),
	}
}

func (nm *NostrManager) AddSubscription(walletServicePubkey string, appPubkey string, relays []string) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	details := &nwc.SubscriptionDetails{
		AppPubkeys: make(map[string]bool),
		Relays:     make(map[string]bool),
	}
	details.AppPubkeys[appPubkey] = true
	for _, relay := range relays {
		details.Relays[relay] = true
	}

	sub, exists := nm.subs[walletServicePubkey]
	if exists {
		for appPubkey := range sub.details.AppPubkeys {
			details.AppPubkeys[appPubkey] = true
		}
		for relay := range sub.details.Relays {
			details.Relays[relay] = true
		}
		sub.cancel()
	}

	nm.addSubscriptionInner(walletServicePubkey, details)
}

func (nm *NostrManager) addSubscriptionInner(walletServicePubkey string, subDetails *nwc.SubscriptionDetails) {
	filters := nostr.Filter{
		Tags: nostr.TagMap{
			"p": []string{walletServicePubkey},
		},
		Kinds: []nostr.Kind{nostr.KindNWCWalletRequest},
	}
	subCtx, subCancel := context.WithCancel(nm.ctx)
	relays := slices.Collect(maps.Keys(subDetails.Relays))
	eventChannel := nm.pool.SubscribeMany(subCtx, relays, filters, nostr.SubscriptionOptions{})
	sub := Subscription{
		ctx:          subCtx,
		cancel:       subCancel,
		eventChannel: eventChannel,
		details:      subDetails,
	}
	nm.subs[walletServicePubkey] = &sub

	log.Printf("Subscribed to %d relays for wallet pubkey %s", len(subDetails.Relays), walletServicePubkey)

	go nm.forwardToNotify(&sub, walletServicePubkey)
}

func (nm *NostrManager) RemoveSubscription(walletServicePubkey string, appPubkey string) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	existingSub, exists := nm.subs[walletServicePubkey]
	if !exists {
		return
	}

	delete(existingSub.details.AppPubkeys, appPubkey)
	if len(existingSub.details.AppPubkeys) == 0 {
		nm.cancelSubscription(existingSub, walletServicePubkey)
		return
	}

	existingSub.cancel()
	nm.addSubscriptionInner(walletServicePubkey, existingSub.details)
}

func (nm *NostrManager) forwardToNotify(sub *Subscription, walletServicePubkey string) {
	if sub == nil {
		return
	}

	for {
		select {
		case incomingEvent := <-sub.eventChannel:
			eventId := incomingEvent.ID.Hex()
			eventAuthor := incomingEvent.PubKey.Hex()

			if _, exists := sub.details.AppPubkeys[eventAuthor]; !exists {
				continue
			}
			if !incomingEvent.VerifySignature() {
				log.Printf("failed to verify signature for event %v", eventId)
				continue
			}
			log.Printf("got incoming event: %s", eventId)

			// Check if event has already been forwarded (deduplication)
			alreadyForwarded, err := nm.store.Nwc.IsEventForwarded(sub.ctx, eventId)
			if err != nil {
				log.Printf("failed to check if event %v was already forwarded: %s", eventId, err)
				continue
			}
			if alreadyForwarded {
				log.Printf("event %v already forwarded, skipping duplicate", eventId)
				continue
			}

			webhook, err := nm.store.Nwc.Get(sub.ctx, walletServicePubkey, eventAuthor)
			if err != nil {
				log.Printf("failed to retrieve webhook for event %v: %v", eventId, err)
				continue
			}
			if webhook == nil {
				log.Printf("webhook not found for event %v. Skipping.", eventId)
				continue
			}

			go func(url string, event nostr.RelayEvent, walletServicePk string) {
				eventId := incomingEvent.ID.Hex()
				appPubkey := incomingEvent.PubKey.Hex()

				log.Printf("forwarding event %s to notify service", eventId)

				eventJson, err := event.MarshalJSON()
				if err != nil {
					log.Printf("failed to json-encode event %s: %v", eventId, err)
					return
				}

				err = nm.SendRequest(sub.ctx, url, string(eventJson), eventId)
				if err != nil {
					log.Printf("failed to send webhook message for event %v: %v", eventId, err)
					return
				}

				err = nm.store.Nwc.Update(sub.ctx, nwc.WebhookDetails{
					EventId:             eventId,
					WalletServicePubkey: walletServicePk,
					AppPubkey:           appPubkey,
					WebhookUrl:          url,
				})
				if err != nil {
					log.Printf("failed to update webhook details for event %v: %v", eventId, err)
				}
			}(webhook.Url, incomingEvent, walletServicePubkey)
		case <-sub.ctx.Done():
			return
		case <-nm.ctx.Done():
			return
		}
	}
}

func (nm *NostrManager) SendRequest(ctx context.Context, url string, rawEvent string, eventId string) error {
	message := channel.WebhookMessage{
		Template: "nwc_event",
		Data: map[string]any{
			"event": rawEvent,
		},
	}
	jsonBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(jsonBytes)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status: %d", res.StatusCode)
	}

	log.Printf("successfully forwarded event %s", eventId)
	return nil
}

func (nm *NostrManager) Start() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if nm.isRunning {
		return nil
	}
	nm.ctx, nm.cancel = context.WithCancel(context.Background())
	nm.pool = nostr.NewPool(nostr.PoolOptions{})
	nm.isRunning = true

	activeSubscriptions, err := nm.store.Nwc.GetSubscriptionDetails(nm.ctx)
	if err != nil {
		return err
	}

	for walletServicePubkey, subDetails := range activeSubscriptions {
		nm.addSubscriptionInner(walletServicePubkey, &subDetails)
	}

	log.Printf("Started Nostr manager")
	return nil
}

func (nm *NostrManager) Stop() {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if !nm.isRunning {
		return
	}
	for walletServicePubkey, sub := range nm.subs {
		nm.cancelSubscription(sub, walletServicePubkey)
	}

	if nm.cancel != nil {
		nm.cancel()
	}

	nm.isRunning = false
	log.Printf("Stopped Nostr manager")
}

func (nm *NostrManager) cancelSubscription(s *Subscription, walletServicePubkey string) {
	s.cancel()
	delete(nm.subs, walletServicePubkey)
}
