package persist

import (
	"context"
	"fmt"
	"time"
)

type MemoryStore struct {
	webhooks        []Webhook
	forwardedEvents map[string]bool // eventId -> forwarded
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		webhooks:        []Webhook{},
		forwardedEvents: make(map[string]bool),
	}
}

func (m *MemoryStore) Set(ctx context.Context, webhook Webhook) error {
	for i, hook := range m.webhooks {
		if hook.Compare(webhook.WalletServicePubkey, webhook.AppPubkey) {
			m.webhooks[i] = webhook
			return nil
		}
	}
	m.webhooks = append(m.webhooks, webhook)
	return nil
}

func (m *MemoryStore) Get(ctx context.Context, walletServicePubkey string, appPubkey string) (*Webhook, error) {
	for _, hook := range m.webhooks {
		if hook.Compare(walletServicePubkey, appPubkey) {
			return &hook, nil
		}
	}
	return nil, fmt.Errorf("Webhook not found")
}

func (m *MemoryStore) Delete(ctx context.Context, walletServicePubkey string, appPubkey string) error {
	for i, hook := range m.webhooks {
		if hook.Compare(walletServicePubkey, appPubkey) {
			m.webhooks = append(m.webhooks[:i], m.webhooks[i+1:]...)
		}
	}
	return nil
}

func (m *MemoryStore) Update(ctx context.Context, details WebhookDetails) error {
	m.forwardedEvents[details.EventId] = true
	now := time.Now()
	for i, hook := range m.webhooks {
		if hook.Compare(details.WalletServicePubkey, details.AppPubkey) {
			m.webhooks[i].LastUsedAt = &now
			return nil
		}
	}
	return nil
}

func (m *MemoryStore) GetSubscriptionDetails(ctx context.Context) (map[string]SubscriptionDetails, error) {
	subs := make(map[string]SubscriptionDetails)
	for _, hook := range m.webhooks {
		sub, ok := subs[hook.WalletServicePubkey]
		if !ok {
			sub = SubscriptionDetails{
				AppPubkeys: make(map[string]bool),
				Relays:     make(map[string]bool),
			}
		}
		sub.AppPubkeys[hook.AppPubkey] = true
		for _, relay := range hook.Relays {
			sub.Relays[relay] = true
		}
		subs[hook.WalletServicePubkey] = sub
	}
	return subs, nil
}

func (m *MemoryStore) GetRelays(ctx context.Context) ([]string, error) {
	relays := make(map[string]bool)
	for _, hook := range m.webhooks {
		for _, relay := range hook.Relays {
			relays[relay] = true
		}
	}
	var result []string
	for relay := range relays {
		result = append(result, relay)
	}
	return result, nil
}

func (m *MemoryStore) DeleteExpired(ctx context.Context, before time.Time) error {
	return nil
}

func (m *MemoryStore) IsEventForwarded(ctx context.Context, eventId string) (bool, error) {
	return m.forwardedEvents[eventId], nil
}
func (m *MemoryStore) DeleteOldForwardedEvents(ctx context.Context, before time.Time) error {
	// In-memory implementation doesn't need cleanup as it's temporary
	return nil
}
