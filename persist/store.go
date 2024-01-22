package persist

import (
	"context"
	"time"
)

type Webhook struct {
	Pubkey string `json:"pubkey" db:"pubkey"`
	Url    string `json:"url" db:"url"`
}

type Store interface {
	Set(ctx context.Context, webhook Webhook) error
	GetLastUpdated(ctx context.Context, pubkey string) (*Webhook, error)
	Remove(ctx context.Context, pubkey, url string) error
	DeleteExpired(ctx context.Context, before time.Time) error
}

type MemoryStore struct {
	webhooks []Webhook
}

func (m *MemoryStore) Set(ctx context.Context, webhook Webhook) error {
	var hooks []Webhook
	for _, hook := range m.webhooks {
		if hook.Pubkey == webhook.Pubkey && hook.Url == webhook.Url {
			continue
		}
		hooks = append(hooks, hook)
	}
	m.webhooks = append([]Webhook{webhook}, hooks...)
	return nil
}

func (m *MemoryStore) GetLastUpdated(ctx context.Context, pubkey string) (*Webhook, error) {
	for _, hook := range m.webhooks {
		if hook.Pubkey == pubkey {
			return &hook, nil
		}
	}
	return nil, nil
}
func (m *MemoryStore) Remove(ctx context.Context, pubkey, url string) error {
	var hooks []Webhook
	for _, hook := range m.webhooks {
		if hook.Pubkey == pubkey && hook.Url == url {
			continue
		}
		hooks = append(hooks, hook)
	}
	m.webhooks = hooks
	return nil
}

func (m *MemoryStore) DeleteExpired(ctx context.Context, before time.Time) error {
	return nil
}
