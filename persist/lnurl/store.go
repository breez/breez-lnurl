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
