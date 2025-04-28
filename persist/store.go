package persist

import (
	"context"
	"time"
)

type Webhook struct {
	Pubkey   string  `json:"pubkey" db:"pubkey"`
	Url      string  `json:"url" db:"url"`
	Username *string `json:"username" db:"username"`
	Offer    *string `json:"offer" db:"offer"`
}

type PubkeyDetails struct {
	Pubkey   string  `json:"pubkey" db:"pubkey"`
	Username string  `json:"username" db:"username"`
	Offer    *string `json:"offer" db:"offer"`
}

func (w Webhook) Compare(identifier string) bool {
	if w.Pubkey == identifier {
		return true
	}

	if w.Username == nil {
		return false
	}

	return *w.Username == identifier
}

type Store interface {
	Set(ctx context.Context, webhook Webhook) (*Webhook, error)
	SetPubkeyDetails(ctx context.Context, pubkey string, username string, offer *string) (*PubkeyDetails, error)
	GetLastUpdated(ctx context.Context, identifier string) (*Webhook, error)
	GetPubkeyDetails(ctx context.Context, identifier string) (*PubkeyDetails, error)
	Remove(ctx context.Context, pubkey, url string) error
	DeleteExpired(ctx context.Context, before time.Time) error
}

type MemoryStore struct {
	webhooks []Webhook
}

func (m *MemoryStore) Set(ctx context.Context, webhook Webhook) (*Webhook, error) {
	var hooks []Webhook
	for _, hook := range m.webhooks {
		if hook.Pubkey == webhook.Pubkey && hook.Url == webhook.Url {
			continue
		}
		hooks = append(hooks, hook)
	}
	m.webhooks = append([]Webhook{webhook}, hooks...)
	return &webhook, nil
}

func (m *MemoryStore) SetPubkeyDetails(ctx context.Context, pubkey string, username string, offer *string) (*PubkeyDetails, error) {
	var hooks []Webhook
	var webhook Webhook
	for _, hook := range m.webhooks {
		if hook.Pubkey == pubkey {
			webhook = hook
			continue
		}
		hooks = append(hooks, hook)
	}

	webhook.Pubkey = pubkey
	webhook.Username = &username
	webhook.Offer = offer
	m.webhooks = append([]Webhook{webhook}, hooks...)
	return &PubkeyDetails{
		Pubkey:   webhook.Pubkey,
		Username: username,
		Offer:    offer,
	}, nil
}

func (m *MemoryStore) GetLastUpdated(ctx context.Context, identifier string) (*Webhook, error) {
	for _, hook := range m.webhooks {
		if hook.Compare(identifier) {
			return &hook, nil
		}
	}
	return nil, nil
}

func (m *MemoryStore) GetPubkeyDetails(ctx context.Context, identifier string) (*PubkeyDetails, error) {
	for _, hook := range m.webhooks {
		if hook.Compare(identifier) {
			if hook.Username != nil {
				return &PubkeyDetails{
					Pubkey:   hook.Pubkey,
					Username: *hook.Username,
					Offer:    hook.Offer,
				}, nil
			}
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
