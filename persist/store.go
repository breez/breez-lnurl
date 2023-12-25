package persist

import (
	"context"
	"time"
)

type Webhook struct {
	Pubkey string `json:"pubkey"`
	HookID string `json:"hook_id"`
	Url    string `json:"url"`
}

type Store interface {
	Set(ctx context.Context, webhook Webhook) error
	Get(ctx context.Context, pubkey, appKey string) (*Webhook, error)
	DeleteExpired(ctx context.Context, before time.Time) error
}

type MemoryStore struct {
	webhooks []Webhook
}

func (m *MemoryStore) Set(ctx context.Context, webhook Webhook) error {
	var hooks []Webhook
	for _, hook := range m.webhooks {
		if hook.Pubkey == webhook.Pubkey && hook.HookID == webhook.HookID {
			continue
		}
		hooks = append(hooks, hook)
	}
	m.webhooks = append(hooks, webhook)
	return nil
}

func (m *MemoryStore) Get(ctx context.Context, pubkey, hookID string) (*Webhook, error) {
	for _, hook := range m.webhooks {
		if hook.Pubkey == pubkey && hook.HookID == hookID {
			return &hook, nil
		}
	}
	return nil, nil
}

func (m *MemoryStore) DeleteExpired(ctx context.Context, before time.Time) error {
	return nil
}
