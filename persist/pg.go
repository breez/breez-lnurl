package persist

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PgStore struct {
	pool *pgxpool.Pool
}

func NewPgStore(databaseUrl string) (*PgStore, error) {
	pool, err := pgConnect(databaseUrl)
	if err != nil {
		return nil, fmt.Errorf("pgConnect() error: %v", err)
	}
	return &PgStore{pool: pool}, nil
}

func (s *PgStore) Set(ctx context.Context, webhook Webhook) (*Webhook, error) {
	pk, err := hex.DecodeString(webhook.Pubkey)
	if err != nil {
		return nil, err
	}
	if webhook.Username != nil {
		username := strings.ToLower(*webhook.Username)
		_, err := s.SetPubkeyDetails(ctx, webhook.Pubkey, username, webhook.Offer)
		if err != nil {
			return nil, err
		}
		webhook.Username = &username
	}

	now := time.Now().UnixMicro()
	res, err := s.pool.Exec(
		ctx,
		`INSERT INTO public.lnurl_webhooks (pubkey, url, created_at, refreshed_at)
		 values ($1, $2, $3, $4)		 
		 ON CONFLICT (pubkey, url) DO UPDATE SET url=$2, refreshed_at = $4`,
		pk,
		webhook.Url,
		now,
		now,
	)
	if err != nil {
		return nil, err
	}
	if res.RowsAffected() == 0 {
		return nil, fmt.Errorf("failed to set webhook for pubkey: %v", webhook.Pubkey)
	}
	return &webhook, err
}

func (s *PgStore) SetPubkeyDetails(ctx context.Context, pubkey string, username string, offer *string) (*PubkeyDetails, error) {
	pk, err := hex.DecodeString(pubkey)
	if err != nil {
		return nil, err
	}
	username = strings.ToLower(username)
	res, err := s.pool.Exec(
		ctx,
		`INSERT INTO public.pubkey_details (pubkey, username, offer) 
		 values ($1, $2, $3)
		 ON CONFLICT (pubkey) DO UPDATE SET username = $2, offer = $3`,
		pk,
		username,
		offer,
	)
	if err != nil {
		return nil, NewErrorUsernameConflict(username, err)
	}

	if res.RowsAffected() == 0 {
		return nil, fmt.Errorf("failed to set offer for pubkey: %v", pubkey)
	}
	return &PubkeyDetails{
		Pubkey:   pubkey,
		Username: username,
		Offer:    offer,
	}, nil
}

func (s *PgStore) GetLastUpdated(ctx context.Context, identifier string) (*Webhook, error) {
	pk := decodeIdentifier(identifier)

	// Get the webhook record by the identifier which can either a decoded pubkey or username.
	rows, err := s.pool.Query(
		ctx,
		`SELECT encode(lw.pubkey, 'hex') pubkey, lw.url, lpu.username, lpu.offer
		 FROM public.lnurl_webhooks lw
         LEFT JOIN public.pubkey_details lpu ON lw.pubkey = lpu.pubkey
		 WHERE lw.pubkey = $1 OR lpu.username = $2
		 ORDER BY lw.refreshed_at DESC LIMIT 1`,
		pk,
		strings.ToLower(identifier),
	)

	if err != nil {
		return nil, err
	}
	defer rows.Close()
	webhooks, err := pgx.CollectRows(rows, pgx.RowToStructByName[Webhook])
	if err != nil {
		return nil, err
	}
	if len(webhooks) != 1 {
		return nil, fmt.Errorf("unexpected webhooks count for: %v", identifier)
	}
	return &webhooks[0], nil
}

func (s *PgStore) GetPubkeyDetails(ctx context.Context, identifier string) (*PubkeyDetails, error) {
	pk := decodeIdentifier(identifier)

	// Get the pubkey usernames record by the identifier which can either a decoded pubkey or username.
	rows, err := s.pool.Query(
		ctx,
		`SELECT encode(lpu.pubkey, 'hex') pubkey, lpu.username, lpu.offer 
		 FROM public.pubkey_details lpu
		 WHERE lpu.pubkey = $1 OR lpu.username = $2
		 LIMIT 1`,
		pk,
		strings.ToLower(identifier),
	)

	if err != nil {
		return nil, err
	}
	defer rows.Close()
	PubkeyDetailss, err := pgx.CollectRows(rows, pgx.RowToStructByName[PubkeyDetails])
	if err != nil {
		return nil, err
	}
	if len(PubkeyDetailss) != 1 {
		return nil, fmt.Errorf("unexpected pubkey usernames count for: %v count: %v", identifier, len(PubkeyDetailss))
	}
	return &PubkeyDetailss[0], nil
}

func (s *PgStore) Remove(ctx context.Context, pubkey, url string) error {
	pk, err := hex.DecodeString(pubkey)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(
		ctx,
		`DELETE FROM public.lnurl_webhooks
		 WHERE pubkey = $1 and url = $2`,
		pk,
		url,
	)

	return err
}

func (s *PgStore) DeleteExpired(
	ctx context.Context,
	before time.Time,
) error {
	_, err := s.pool.Exec(
		ctx,
		`DELETE FROM public.lnurl_webhooks
		 WHERE refreshed_at < $1`,
		before.UnixMicro())

	return err
}

func pgConnect(databaseUrl string) (*pgxpool.Pool, error) {
	pgxPool, err := pgxpool.New(context.Background(), databaseUrl)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New(%v): %w", databaseUrl, err)
	}
	return pgxPool, nil
}

func decodeIdentifier(identifier string) *[]byte {
	pk, err := hex.DecodeString(identifier)
	if err != nil {
		return nil
	}

	return &pk
}
