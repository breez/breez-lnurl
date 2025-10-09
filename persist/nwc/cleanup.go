package persist

import (
	"context"
	"log"
	"time"
)

type CleanupService struct {
	store     Store
	channel   chan struct{}
	callbacks [](func() error)
}

// The interval to clean expired listeners
var CleanupInterval time.Duration = 10 * time.Minute

// The expiry duration is the time until a non-refreshed listener expires
var ExpiryDuration time.Duration = time.Hour * 24 * 7

func NewCleanupService(store Store) *CleanupService {
	return &CleanupService{
		store:     store,
		channel:   make(chan struct{}),
		callbacks: [](func() error){},
	}
}

// Periodically cleans up expired NWC uris
func (c *CleanupService) Start(ctx context.Context) {
	for {
		before := time.Now().Add(-ExpiryDuration)
		err := c.store.DeleteExpired(ctx, before)
		if err != nil {
			log.Printf("Failed to remove expired listeners before %v: %v", before, err)
		}
		for _, cb := range c.callbacks {
			if err := cb(); err != nil {
				log.Printf("Failed to run cleanup callback: %v", err)
			}
		}
		select {
		case <-time.After(CleanupInterval):
			continue
		case <-ctx.Done():
			return
		}
	}
}

func (c *CleanupService) OnCleanup(cb func() error) {
	c.callbacks = append(c.callbacks, cb)
}
