package persist

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/breez/breez-lnurl/constant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PgStore struct {
	pool *pgxpool.Pool
}

func NewPgStore(pool *pgxpool.Pool) *PgStore {
	return &PgStore{
		pool,
	}
}

func (s *PgStore) Set(ctx context.Context, webhook Webhook) error {
	walletServicePubkey, err := hex.DecodeString(webhook.WalletServicePubkey)
	if err != nil {
		return err
	}
	appPubkey, err := hex.DecodeString(webhook.AppPubkey)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var webhookId int64
	err = tx.QueryRow(
		ctx,
		`INSERT INTO public.nwc_webhooks (url, wallet_service_pubkey, app_pubkey, updated_at)
		 VALUES ($1, $2, $3, NOW())
		 ON CONFLICT (wallet_service_pubkey, app_pubkey) DO UPDATE SET url = $1, updated_at = NOW()
		 RETURNING id`,
		webhook.Url,
		walletServicePubkey,
		appPubkey,
	).Scan(&webhookId)
	if err != nil {
		return fmt.Errorf("failed to insert/update webhook: %w", err)
	}

	relays, err := getRelaysByUrl(ctx, tx)
	if err != nil {
		return err
	}

	for _, relayUrl := range webhook.Relays {
		relayId, exists := relays[relayUrl]
		if !exists {
			relayId = len(relays) % constant.NWC_MAX_RELAYS_LENGTH
			_, err = tx.Exec(
				ctx,
				`INSERT INTO public.nwc_relays (id, url)
                 VALUES ($1, $2)
                 ON CONFLICT (id) DO UPDATE SET url = EXCLUDED.url`,
				relayId, relayUrl,
			)
			if err != nil {
				return fmt.Errorf("failed to insert relay: %w", err)
			}
			relays[relayUrl] = relayId
		}

		_, err = tx.Exec(
			ctx,
			`INSERT INTO public.nwc_webhooks_relays (webhook_id, relay_id)
		 	 VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			webhookId,
			relayId,
		)
		if err != nil {
			return fmt.Errorf("failed to link webhook and relay: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (s *PgStore) Get(ctx context.Context, walletServicePubkey string, appPubkey string) (*Webhook, error) {
	walletServicePubkeyBytes, err := hex.DecodeString(walletServicePubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid wallet service pubkey: %w", err)
	}
	appPubkeyBytes, err := hex.DecodeString(appPubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid app pubkey: %w", err)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var webhookId int64
	var url string
	err = tx.QueryRow(
		ctx,
		`SELECT id, url 
		 FROM public.nwc_webhooks 
		 WHERE wallet_service_pubkey = $1 AND app_pubkey = $2`,
		walletServicePubkeyBytes,
		appPubkeyBytes,
	).Scan(&webhookId, &url)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("querying webhook: %w", err)
	}

	rows, err := tx.Query(
		ctx,
		`SELECT nr.url
		 FROM public.nwc_webhooks_relays nwr
		 INNER JOIN public.nwc_relays nr ON nwr.relay_id = nr.id
		 WHERE nwr.webhook_id = $1`,
		webhookId,
	)
	if err != nil {
		return nil, fmt.Errorf("querying relays: %w", err)
	}
	defer rows.Close()

	var relays []string
	for rows.Next() {
		var relayUrl string
		if err := rows.Scan(&relayUrl); err != nil {
			return nil, err
		}
		relays = append(relays, relayUrl)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &Webhook{
		Relays:              relays,
		AppPubkey:           appPubkey,
		WalletServicePubkey: walletServicePubkey,
		Url:                 url,
	}, nil
}

func (s *PgStore) Delete(ctx context.Context, walletServicePubkey string, appPubkey string) error {
	_, err := s.pool.Exec(
		ctx,
		`DELETE FROM public.nwc_webhooks WHERE wallet_service_pubkey = $1 AND app_pubkey = $2`,
		walletServicePubkey,
		appPubkey,
	)
	if err != nil {
		return err
	}
	return nil
}

func (s *PgStore) GetSubscriptionDetails(ctx context.Context) (map[string]SubscriptionDetails, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT encode(w.wallet_service_pubkey, 'hex'), encode(w.app_pubkey, 'hex'), nr.url
		 FROM public.nwc_webhooks w
		 LEFT JOIN public.nwc_webhooks_relays nwr ON w.id = nwr.webhook_id
		 LEFT JOIN public.nwc_relays nr ON nwr.relay_id = nr.id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	subs := make(map[string]SubscriptionDetails)
	for rows.Next() {
		var walletServicePubkey, appPubkey string
		var relayUrl *string
		if err := rows.Scan(&walletServicePubkey, &appPubkey, &relayUrl); err != nil {
			return nil, err
		}
		sub, ok := subs[walletServicePubkey]
		if !ok {
			sub = SubscriptionDetails{
				AppPubkeys: make(map[string]bool),
				Relays:     make(map[string]bool),
			}
		}
		sub.AppPubkeys[appPubkey] = true
		if relayUrl != nil {
			sub.Relays[*relayUrl] = true
		}
		subs[walletServicePubkey] = sub
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return subs, nil
}

func (s *PgStore) GetRelays(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT url FROM public.nwc_relays`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return rowsToArray(rows), nil
}

func (s *PgStore) DeleteExpired(ctx context.Context, before time.Time) error {
	beforeUnix := before.Unix()
	_, err := s.pool.Exec(
		ctx,
		`DELETE FROM public.nwc_webhooks
		 WHERE updated_at < to_timestamp($1)`,
		beforeUnix)
	return err
}

func getRelaysByUrl(ctx context.Context, con pgx.Tx) (map[string]int, error) {
	rows, err := con.Query(ctx, `SELECT id, url FROM public.nwc_relays`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var id int
	var url string
	relays := make(map[string]int)
	for rows.Next() {
		err := rows.Scan(&id, &url)
		if err != nil {
			return nil, err
		}
		relays[url] = id
	}
	return relays, nil
}

func rowsToArray(rows pgx.Rows) []string {
	arr := []string{}
	for rows.Next() {
		var val string
		rows.Scan(&val)
		arr = append(arr, val)
	}
	return arr
}

func (s *PgStore) IsEventForwarded(ctx context.Context, eventId string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(
		ctx,
		`SELECT EXISTS(SELECT 1 FROM public.nwc_forwarded_events WHERE event_id = $1)`,
		eventId,
	).Scan(&exists)

	if err != nil {
		return false, fmt.Errorf("failed to check if event is forwarded: %w", err)
	}

	return exists, nil
}

func (s *PgStore) MarkEventForwarded(ctx context.Context, eventId string, walletServicePubkey string, appPubkey string, webhookUrl string) error {
	walletServicePubkeyBytes, err := hex.DecodeString(walletServicePubkey)
	if err != nil {
		return fmt.Errorf("failed to decode wallet service pubkey: %w", err)
	}

	appPubkeyBytes, err := hex.DecodeString(appPubkey)
	if err != nil {
		return fmt.Errorf("failed to decode app pubkey: %w", err)
	}

	_, err = s.pool.Exec(
		ctx,
		`INSERT INTO public.nwc_forwarded_events (event_id, wallet_service_pubkey, app_pubkey, webhook_url, forwarded_at)
		 VALUES ($1, $2, $3, $4, NOW())
		 ON CONFLICT (event_id) DO NOTHING`,
		eventId,
		walletServicePubkeyBytes,
		appPubkeyBytes,
		webhookUrl,
	)

	if err != nil {
		return fmt.Errorf("failed to mark event as forwarded: %w", err)
	}

	return nil
}

func (s *PgStore) DeleteOldForwardedEvents(ctx context.Context, before time.Time) error {
	beforeUnix := before.Unix()
	_, err := s.pool.Exec(
		ctx,
		`DELETE FROM public.nwc_forwarded_events
		 WHERE forwarded_at < to_timestamp($1)`,
		beforeUnix,
	)
	return err
}
