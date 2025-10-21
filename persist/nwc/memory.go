package persist

import (
	"context"
	"fmt"
	"time"
)

type MemoryStore struct {
	webhooks []Webhook
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		webhooks: []Webhook{},
	}
}

func (m *MemoryStore) Set(ctx context.Context, webhook Webhook) error {
	for _, hook := range m.webhooks {
		if hook.Compare(webhook.UserPubkey, webhook.AppPubkey) {
			return nil
		}
	}
	m.webhooks = append([]Webhook{webhook}, webhook)
	return nil
}

func (m *MemoryStore) Get(ctx context.Context, userPubkey string, appPubkey string) (*Webhook, error) {
	for _, hook := range m.webhooks {
		if hook.Compare(userPubkey, appPubkey) {
			return &hook, nil
		}
	}
	return nil, fmt.Errorf("Webhook not found")
}

func (m *MemoryStore) Delete(ctx context.Context, userPubkey string, appPubkey string) error {
	for i, hook := range m.webhooks {
		if hook.Compare(userPubkey, appPubkey) {
			m.webhooks = append(m.webhooks[:i], m.webhooks[i+1:]...)
		}
	}
	return nil
}

func (m *MemoryStore) GetAppPubkeys(ctx context.Context) ([]string, error) {
	var pubkeys []string
	for _, hook := range m.webhooks {
		pubkeys = append(pubkeys, hook.AppPubkey)
	}
	return pubkeys, nil
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
