package persist

import (
	"context"
	"log"
	"time"
)

type CleanupService struct {
	store   Store
	channel chan struct{}
}

// The interval to clean expired listeners
var CleanupInterval time.Duration = 10 * time.Minute

// The expiry duration is the time until an unused webhook expires
var ExpiryDuration time.Duration = time.Hour * 24 * 30

// The duration to keep forwarded events records (7 days)
var ForwardedEventsRetentionDuration time.Duration = time.Hour * 24 * 7

func NewCleanupService(store Store) *CleanupService {
	return &CleanupService{
		store:   store,
		channel: make(chan struct{}),
	}
}

// Periodically cleans up expired NWC uris and old forwarded events
func (c *CleanupService) Start(ctx context.Context) {
	for {
		// Cleanup expired webhooks
		webhooksBefore := time.Now().Add(-ExpiryDuration)
		err := c.store.DeleteExpired(ctx, webhooksBefore)
		if err != nil {
			log.Printf("Failed to remove expired listeners before %v: %v", webhooksBefore, err)
		}

		// Cleanup old forwarded events records
		eventsBefore := time.Now().Add(-ForwardedEventsRetentionDuration)
		err = c.store.DeleteOldForwardedEvents(ctx, eventsBefore)
		if err != nil {
			log.Printf("Failed to remove old forwarded events before %v: %v", eventsBefore, err)
		}

		select {
		case <-time.After(CleanupInterval):
			continue
		case <-ctx.Done():
			return
		}
	}
}
