package persist

import (
	"fmt"
	"context"
	"github.com/jackc/pgx/v5/pgxpool"

	lnurl "github.com/breez/breez-lnurl/persist/lnurl"
	nwc "github.com/breez/breez-lnurl/persist/nwc"
)

type Store struct {
	LnUrl lnurl.Store
	Nwc  nwc.Store
}

func NewMemoryStore() *Store {
	return &Store {
		LnUrl: lnurl.NewMemoryStore(),
	}
}

func NewPgStore(databaseUrl string) (*Store, error) {
	pool, err := pgConnect(databaseUrl)
	if err != nil {
		return nil, fmt.Errorf("pgConnect() error: %v", err)
	}
	return &Store{
		LnUrl: lnurl.NewPgStore(pool),
		Nwc: nwc.NewPgStore(pool),
	}, nil
}

func pgConnect(databaseUrl string) (*pgxpool.Pool, error) {
	pgxPool, err := pgxpool.New(context.Background(), databaseUrl)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New(%v): %w", databaseUrl, err)
	}
	return pgxPool, nil
}
