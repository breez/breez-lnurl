package persist

import (
	"context"
	"time"
)

type Webhook struct {
	UserPubkey string   `json:"userPubkey" db:"user_pubkey"`
	AppPubkey  string   `json:"appPubkey" db:"app_pubkey"`
	Url        string   `json:"url" db:"url"`
	Relays     []string `json:"relays" db:"relays"`
}

type Store interface {
	Set(ctx context.Context, webhook Webhook) error
	Get(ctx context.Context, userPubkey string, appPubkey string) (*Webhook, error)
	GetAppPubkeys(ctx context.Context) ([]string, error)
	GetRelays(ctx context.Context) ([]string, error)
	DeleteExpired(ctx context.Context, before time.Time) error
}
