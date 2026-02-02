package nwc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/breez/breez-lnurl/channel"
	"github.com/breez/breez-lnurl/persist"
	"github.com/nbd-wtf/go-nostr"
)

type Subscription struct {
	ctx          context.Context
	cancel       context.CancelFunc
	eventChannel chan nostr.IncomingEvent
}

type NostrManager struct {
	pool           *nostr.SimplePool
	ctx            context.Context
	cancel         context.CancelFunc
	mu             sync.RWMutex
	isRunning      bool
	sub            *Subscription
	store          *persist.Store
	lastAppPubkeys []string
}

func NewNostrManager(store *persist.Store) *NostrManager {
	return &NostrManager{
		isRunning: false,
		store:     store,
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

func (nm *NostrManager) Resubscribe() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if !nm.isRunning {
		return fmt.Errorf("manager not running")
	}

	appPubkeys, err := nm.store.Nwc.GetAppPubkeys(nm.ctx)
	if err != nil {
		return err
	}

	// Only resubscribe if we have pubkeys to subscribe to
	if len(appPubkeys) == 0 {
		log.Printf("No active app pubkeys. Waiting for registrations...")
		return nil
	}

	// Only resubscribe if pubkeys have changed to avoid rate limiting
	if nm.lastAppPubkeys != nil && slices.Compare(nm.lastAppPubkeys, appPubkeys) == 0 {
		return nil
	}

	relays, err := nm.store.Nwc.GetRelays(nm.ctx)

	filters := nostr.Filters{
		{
			Authors: appPubkeys,
		},
	}

	prevSub := nm.sub
	subCtx, subCancel := context.WithCancel(nm.ctx)
	nm.sub = &Subscription{
		eventChannel: nm.pool.SubMany(nm.ctx, []string{"wss://nos.lol", "wss://nostr.land"}, filters),
		ctx:          subCtx,
		cancel:       subCancel,
	}
	go nm.forwardToNotify()

	if prevSub != nil {
		prevSub.cancel()
		prevSub.eventChannel = nil
	}

	nm.lastAppPubkeys = appPubkeys
	log.Printf("Resubscribed to %d relays for %d app pubkeys using SimplePool.SubMany", len(relays), len(appPubkeys))
	return nil
}

func (nm *NostrManager) forwardToNotify() {
	sub := nm.sub
	if sub == nil {
		return
	}

	for {
		select {
		case incomingEvent := <-sub.eventChannel:
			if incomingEvent.Event == nil {
				return
			}

			log.Printf("got incoming event: %v", incomingEvent.Event.String())
			if _, err := incomingEvent.CheckSignature(); err != nil {
				log.Printf("failed to verify signature for event %v: %v", incomingEvent.ID, err)
				continue
			}

			pTag := incomingEvent.Tags.GetFirst([]string{"p"})
			if pTag == nil {
				log.Printf("failed to identify user for event %v: no wallet service pubkey provided", incomingEvent.ID)
				continue
			}

			walletServicePubkey := pTag.Value()
			// Check if event has already been forwarded (deduplication)
			alreadyForwarded, err := nm.store.Nwc.IsEventForwarded(sub.ctx, incomingEvent.Event.ID)
			if err != nil {
				log.Printf("failed to check if event %v was already forwarded: %v", incomingEvent.ID, err)
				continue
			}
			if alreadyForwarded {
				log.Printf("event %v already forwarded, skipping duplicate", incomingEvent.ID)
				continue
			}

			webhook, err := nm.store.Nwc.Get(sub.ctx, walletServicePubkey, incomingEvent.PubKey)
			if err != nil {
				log.Printf("failed to retrieve webhook for event %v: %v", incomingEvent.ID, err)
				continue
			}
			if webhook == nil {
				log.Printf("webhook not found for event %v. Skipping.", incomingEvent.ID)
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
			}(webhook.Url, incomingEvent.Event.ID, walletServicePubkey, incomingEvent.PubKey)
		case <-sub.ctx.Done():
			return
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
	nm.pool = nostr.NewSimplePool(nm.ctx)
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
