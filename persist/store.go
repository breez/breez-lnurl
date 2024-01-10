package persist

import (
	"context"
	"time"
)

type Webhook struct {
	Pubkey      string `json:"pubkey" db:"pubkey"`
	HookKeyHash string `json:"hook_key_hash" db:"hook_key_hash"`
	Url         string `json:"url" db:"url"`
}

type Store interface {
	Set(ctx context.Context, webhook Webhook) error
	Get(ctx context.Context, pubkey, hookKey string) (*Webhook, error)
	Remove(ctx context.Context, pubkey, hookKey string) error
	DeleteExpired(ctx context.Context, before time.Time) error
}

type MemoryStore struct {
	webhooks []Webhook
}

func (m *MemoryStore) Set(ctx context.Context, webhook Webhook) error {
	var hooks []Webhook
	for _, hook := range m.webhooks {
		if hook.Pubkey == webhook.Pubkey && hook.HookKeyHash == webhook.HookKeyHash {
			continue
		}
		hooks = append(hooks, hook)
	}
	m.webhooks = append(hooks, webhook)
	return nil
}

func (m *MemoryStore) Get(ctx context.Context, pubkey, hookKeyHash string) (*Webhook, error) {
	for _, hook := range m.webhooks {
		if hook.Pubkey == pubkey && hook.HookKeyHash == hookKeyHash {
			return &hook, nil
		}
	}
	return nil, nil
}
func (m *MemoryStore) Remove(ctx context.Context, pubkey, hookKey string) error {
	var hooks []Webhook
	for _, hook := range m.webhooks {
		if hook.Pubkey == pubkey && hook.HookKeyHash == hookKey {
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
