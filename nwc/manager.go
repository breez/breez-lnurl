package nwc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"fiatjaf.com/nostr"
	"github.com/breez/breez-lnurl/channel"
	"github.com/breez/breez-lnurl/persist"
)

type Subscription struct {
	ctx          context.Context
	cancel       context.CancelFunc
	eventChannel chan nostr.RelayEvent
}

type NostrManager struct {
	pool       *nostr.Pool
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex
	isRunning  bool
	sub        *Subscription
	store      *persist.Store
	hasChanged bool
}

func NewNostrManager(store *persist.Store) *NostrManager {
	return &NostrManager{
		isRunning:  false,
		store:      store,
		hasChanged: true,
	}
}

// The interval to check if resubscription is needed
// Only resubscribe if pubkeys have changed to avoid rate limiting
var ResubscribeInterval time.Duration = 1 * time.Minute

func (nm *NostrManager) StartResubscriptionLoop() {
	for {
		if err := nm.Resubscribe(); err != nil {
			log.Printf("failed to resubscribe to events: %v", err)
		}
		select {
		case <-time.After(ResubscribeInterval):
			continue
		case <-nm.ctx.Done():
			return
		}
	}
}

func (nm *NostrManager) SetChanged(changed bool) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	nm.hasChanged = changed
}

func (nm *NostrManager) Resubscribe() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if !nm.isRunning {
		return fmt.Errorf("manager not running")
	}

	activeSubscriptions, err := nm.store.Nwc.GetSubscriptions(nm.ctx)
	if err != nil {
		return err
	}

	// Only resubscribe if we have pubkeys to subscribe to
	if len(activeSubscriptions) == 0 {
		log.Printf("No active subscriptions. Waiting for registrations...")
		return nil
	}

	// Only resubscribe if pubkeys have changed to avoid rate limiting
	if !nm.hasChanged {
		return nil
	}

	relays, err := nm.store.Nwc.GetRelays(nm.ctx)
	filters := nostr.Filter{
		Tags: nostr.TagMap{
			"p": slices.Collect(maps.Keys(activeSubscriptions)),
		},
		Kinds: []nostr.Kind{nostr.KindNWCWalletRequest, nostr.KindZapRequest},
	}
	prevSub := nm.sub
	subCtx, subCancel := context.WithCancel(nm.ctx)
	eventChannel, closeChannel := nm.pool.SubscribeManyNotifyClosed(subCtx, relays, filters, nostr.SubscriptionOptions{})

	nm.sub = &Subscription{
		eventChannel: eventChannel,
		ctx:          subCtx,
		cancel:       subCancel,
	}
	go nm.trackRelayClose(closeChannel)
	go nm.forwardToNotify(activeSubscriptions)
	if prevSub != nil {
		prevSub.cancel()
		prevSub.eventChannel = nil
	}

	nm.hasChanged = false
	log.Printf("Resubscribed to %d relays for %d user pubkeys using SimplePool.SubscribeMany", len(relays), len(activeSubscriptions))
	return nil
}

func (nm *NostrManager) trackRelayClose(closingChan chan nostr.RelayClosed) {
	sub := nm.sub
	if sub == nil {
		return
	}
	for {
		select {
		case close := <-closingChan:
			log.Printf("Received CLOSE from %s - Reason: %s\n", close.Relay.URL, close.Reason)
		case <-sub.ctx.Done():
		case <-nm.ctx.Done():
			return
		}
	}
}

func (nm *NostrManager) forwardToNotify(activeSubscriptions map[string][]string) {
	sub := nm.sub
	if sub == nil {
		return
	}
	for {
		select {
		case incomingEvent := <-sub.eventChannel:
			eventId := incomingEvent.ID.Hex()
			eventAuthor := incomingEvent.PubKey.Hex()

			pTag := incomingEvent.Tags.Find("p")
			if pTag == nil {
				log.Printf("failed to identify user for event %s: no wallet service pubkey provided", eventId)
				continue
			}
			walletServicePubkey := pTag[1]
			if walletServicePubkey == "" {
				continue
			}
			appPubkeys, exists := activeSubscriptions[walletServicePubkey]
			if !exists || slices.Index(appPubkeys, eventAuthor) == -1 {
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

			go func(url string, id string, walletServicePk string, appPk string) {
				log.Printf("forwarding event %s to notify service", id)
				err = nm.SendRequest(sub.ctx, url, id)
				if err != nil {
					log.Printf("failed to send webhook message for event %v: %v", id, err)
					return
				}

				// Mark event as forwarded after successful delivery
				err = nm.store.Nwc.MarkEventForwarded(sub.ctx, id, walletServicePk, appPk, url)
				if err != nil {
					log.Printf("failed to mark event %v as forwarded: %v", id, err)
				}
			}(webhook.Url, eventId, walletServicePubkey, eventAuthor)
		case <-sub.ctx.Done():
		case <-nm.ctx.Done():
			return
		}
	}
}

func (nm *NostrManager) SendRequest(ctx context.Context, url string, eventId string) error {
	message := channel.WebhookMessage{
		Template: "nwc_event",
		Data: map[string]any{
			"event_id": eventId,
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

func (nm *NostrManager) Start() {
	nm.mu.Lock()

	if nm.isRunning {
		return
	}
	nm.ctx, nm.cancel = context.WithCancel(context.Background())
	nm.pool = nostr.NewPool(nostr.PoolOptions{})
	nm.isRunning = true
	log.Printf("NostrManager started with SimplePool")

	nm.mu.Unlock()
	go nm.StartResubscriptionLoop()
}

func (nm *NostrManager) Stop() {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if !nm.isRunning {
		return
	}

	if nm.sub != nil {
		nm.cancelSubscription()
	}

	if nm.cancel != nil {
		nm.cancel()
	}

	nm.isRunning = false
	log.Printf("NostrManager stopped")
}

func (nm *NostrManager) cancelSubscription() {
	nm.sub.cancel()
	close(nm.sub.eventChannel)
	nm.sub.eventChannel = nil
	nm.sub = nil
}
