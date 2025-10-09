package nwc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

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
	pool      *nostr.SimplePool
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.RWMutex
	isRunning bool
	sub       *Subscription
	store     *persist.Store
}

func NewNostrManager(store *persist.Store) *NostrManager {
	return &NostrManager{
		isRunning: false,
		store:     store,
	}
}

func (nm *NostrManager) Resubscribe() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if !nm.isRunning {
		return fmt.Errorf("manager not running")
	}

	if nm.sub != nil {
		nm.cancelSubscription()
	}

	appPubkeys, err := nm.store.Nwc.GetAppPubkeys(nm.ctx)
	if err != nil {
		return err
	}
	relays, err := nm.store.Nwc.GetRelays(nm.ctx)

	filters := nostr.Filters{{
		Authors: appPubkeys,
	}}

	subCtx, subCancel := context.WithCancel(nm.ctx)
	nm.sub = &Subscription{
		eventChannel: nm.pool.SubMany(nm.ctx, relays, filters),
		ctx:          subCtx,
		cancel:       subCancel,
	}
	go nm.forwardToNotify()

	log.Printf("Resubscribed to %d relays for %d pubkeys using SimplePool.SubMany", len(relays), len(appPubkeys))
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
			if _, err := incomingEvent.CheckSignature(); err != nil {
				log.Printf("failed to verify signature for event %v: %v", incomingEvent.ID, err)
				continue
			}

			pTag := incomingEvent.Tags.GetFirst([]string{"p"})
			userPubkey := pTag.Value()
			if userPubkey == "" {
				log.Printf("failed to identify user for event %v: no user pubkey provided", incomingEvent.ID)
				continue
			}

			webhook, err := nm.store.Nwc.Get(sub.ctx, userPubkey, incomingEvent.PubKey)
			if err != nil {
				log.Printf("failed to retrieve webhook for event %v: %v", incomingEvent.ID, err)
				continue
			}

			go func() {
				err = nm.SendRequest(sub.ctx, webhook.Url, incomingEvent.ID)
				if err != nil {
					log.Printf("failed to send webhook message for event %v: %v", incomingEvent.ID, err)
				}
			}()
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
	res, err := http.Post(url, "application/json", strings.NewReader(string(jsonBytes)))
	if err != nil {
		return err
	}
	if res.StatusCode != 200 {
		return errors.New("webhook proxy returned non-200 status code")
	}
	return nil
}

func (nm *NostrManager) Start() error {
	nm.mu.Lock()

	if nm.isRunning {
		return nil
	}
	nm.ctx, nm.cancel = context.WithCancel(context.Background())
	nm.pool = nostr.NewSimplePool(nm.ctx)
	nm.isRunning = true
	log.Printf("NostrManager started with SimplePool")

	nm.mu.Unlock()
	return nm.Resubscribe()
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
