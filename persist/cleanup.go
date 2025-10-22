package persist

import (
	"context"
	lnurl "github.com/breez/breez-lnurl/persist/lnurl"
	nwc "github.com/breez/breez-lnurl/persist/nwc"
)

type CleanupService struct {
	Lnurl *lnurl.CleanupService
	Nwc   *nwc.CleanupService
}

func NewCleanupService(store *Store) *CleanupService {
	return &CleanupService{
		Lnurl: lnurl.NewCleanupService(store.LnUrl),
		Nwc:   nwc.NewCleanupService(store.Nwc),
	}
}

func (c *CleanupService) Start(ctx context.Context) {
	go c.Lnurl.Start(ctx)
	go c.Nwc.Start(ctx)
}
