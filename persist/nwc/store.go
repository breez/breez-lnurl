package persist

import (
	"context"
	"time"
)

type Webhook struct {
	WalletServicePubkey string     `json:"walletServicePubkey" db:"wallet_service_pubkey"`
	AppPubkey           string     `json:"appPubkey" db:"app_pubkey"`
	Url                 string     `json:"url" db:"url"`
	Relays              []string   `json:"relays" db:"relays"`
	LastUsedAt          *time.Time `json:"lastUsedAt" db:"last_used_at"`
}

func (w Webhook) Compare(walletServicePubkey string, appPubkey string) bool {
	return w.AppPubkey == appPubkey && w.WalletServicePubkey == walletServicePubkey
}

type SubscriptionDetails struct {
	AppPubkeys map[string]bool
	Relays     map[string]bool
}

type WebhookDetails struct {
	EventId             string
	WalletServicePubkey string
	AppPubkey           string
	WebhookUrl          string
}

type Store interface {
	Set(ctx context.Context, webhook Webhook) error
	Get(ctx context.Context, walletServicePubkey string, appPubkey string) (*Webhook, error)
	Update(ctx context.Context, details WebhookDetails) error
	Delete(ctx context.Context, walletServicePubkey string, appPubkey string) error
	GetSubscriptionDetails(ctx context.Context) (map[string]SubscriptionDetails, error)
	GetRelays(ctx context.Context) ([]string, error)
	DeleteExpired(ctx context.Context, before time.Time) error
	// Event deduplication methods
	IsEventForwarded(ctx context.Context, eventId string) (bool, error)
	DeleteOldForwardedEvents(ctx context.Context, before time.Time) error
}
