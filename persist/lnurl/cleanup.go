package persist

import (
	"context"
	"log"
	"time"
)

type CleanupService struct {
	store Store
}

// The interval to clean expired webhook urls.
var CleanupInterval time.Duration = time.Hour

// The expiry duration is the time until a non-refreshed webhook url expires.
// Currently set to 30 days.
var ExpiryDuration time.Duration = time.Hour * 24 * 30

func NewCleanupService(store Store) *CleanupService {
	return &CleanupService{
		store: store,
	}
}

// Periodically cleans up expired webhook urls.
func (c *CleanupService) Start(ctx context.Context) {
	for {
		before := time.Now().Add(-ExpiryDuration)
		err := c.store.DeleteExpired(ctx, before)
		if err != nil {
			log.Printf("Failed to remove expired webhook urls before %v: %v", before, err)
		}
		select {
		case <-time.After(CleanupInterval):
			continue
		case <-ctx.Done():
			return
		}
	}
}
