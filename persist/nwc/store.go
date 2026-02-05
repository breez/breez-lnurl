package persist

import (
	"context"
	"time"
)

type Webhook struct {
	WalletServicePubkey string   `json:"walletServicePubkey" db:"wallet_service_pubkey"`
	AppPubkey           string   `json:"appPubkey" db:"app_pubkey"`
	Url                 string   `json:"url" db:"url"`
	Relays              []string `json:"relays" db:"relays"`
}

func (w Webhook) Compare(walletServicePubkey string, appPubkey string) bool {
	return w.AppPubkey == appPubkey && w.WalletServicePubkey == walletServicePubkey
}

type Store interface {
	Set(ctx context.Context, webhook Webhook) error
	Get(ctx context.Context, walletServicePubkey string, appPubkey string) (*Webhook, error)
	Delete(ctx context.Context, walletServicePubkey string, appPubkey string) error
	GetSubscriptions(ctx context.Context) (map[string][]string, error)
	GetRelays(ctx context.Context) ([]string, error)
	DeleteExpired(ctx context.Context, before time.Time) error
	// Event deduplication methods
	IsEventForwarded(ctx context.Context, eventId string) (bool, error)
	MarkEventForwarded(ctx context.Context, eventId string, walletServicePubkey string, appPubkey string, webhookUrl string) error
	DeleteOldForwardedEvents(ctx context.Context, before time.Time) error
}
