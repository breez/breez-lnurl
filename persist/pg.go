package persist

import (
	"context"
	"encoding/hex"
	"fmt"
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

func (s *PgStore) Set(ctx context.Context, webhook Webhook) error {
	pk, err := hex.DecodeString(webhook.Pubkey)
	if err != nil {
		return err
	}

	hookKeyHash, err := hex.DecodeString(webhook.HookKeyHash)
	if err != nil {
		return err
	}

	now := time.Now().UnixMicro()
	_, err = s.pool.Exec(
		ctx,
		`INSERT INTO public.lnurl_webhooks (pubkey, hook_key_hash, url, created_at, refreshed_at)
		 values ($1, $2, $3, $4, $5)
		 ON CONFLICT (pubkey, hook_key_hash) DO UPDATE SET url=$3, refreshed_at = $5`,
		pk,
		hookKeyHash,
		webhook.Url,
		now,
		now,
	)

	return err
}

func (s *PgStore) Get(ctx context.Context, pubkey, hookKeyHash string) (*Webhook, error) {
	pk, err := hex.DecodeString(pubkey)
	if err != nil {
		return nil, err
	}

	rawKeyHash, err := hex.DecodeString(hookKeyHash)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(
		ctx,
		`SELECT encode(pubkey,'hex') pubkey, encode(hook_key_hash,'hex') hook_key_hash, url 
		 FROM public.lnurl_webhooks
		 WHERE pubkey = $1 and hook_key_hash = $2`,
		pk,
		rawKeyHash,
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
		return nil, fmt.Errorf("unexpected webhooks count for pubkey: %v and hook_key_hash: %v", pubkey, hookKeyHash)
	}
	return &webhooks[0], nil
}

func (s *PgStore) Remove(ctx context.Context, pubkey, hookKey string) error {
	pk, err := hex.DecodeString(pubkey)
	if err != nil {
		return err
	}

	hookKeyHash, err := hex.DecodeString(hookKey)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(
		ctx,
		`DELETE FROM public.lnurl_webhooks
		 WHERE pubkey = $1 and hook_key_hash = $2`,
		pk,
		hookKeyHash,
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
