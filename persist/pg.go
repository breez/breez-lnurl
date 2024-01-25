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
		return err
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("failed to set webhook for pubkey: %v", webhook.Pubkey)
	}
	return err
}

func (s *PgStore) GetLastUpdated(ctx context.Context, pubkey string) (*Webhook, error) {
	pk, err := hex.DecodeString(pubkey)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(
		ctx,
		`SELECT encode(pubkey,'hex') pubkey, url 
		 FROM public.lnurl_webhooks
		 WHERE pubkey = $1 order by refreshed_at desc limit 1`,
		pk,
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
		return nil, fmt.Errorf("unexpected webhooks count for pubkey: %v", pubkey)
	}
	return &webhooks[0], nil
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
