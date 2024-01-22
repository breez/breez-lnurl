package persist

import (
	"context"
	"os"
	"testing"
	"time"

	"gotest.tools/assert"
)

func TestPgStore(t *testing.T) {
	pgStore, err := NewPgStore(os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("NewPgStore() error: %v", err)
	}

	assert.NilError(t, pgStore.DeleteExpired(context.Background(), time.Now()), "failed to delete expired")

	// Add a webhook for some pubkey
	assert.NilError(t,
		pgStore.Set(context.Background(), Webhook{
			Pubkey: "02c811e575be2df47d8b48dab3d3f1c9b0f6e16d0d40b5ed78253308fc2bd7170d",
			Url:    "http://example.com",
		}),
		"failed to set webhook")

	// Test that we are able to fetch the right webhook
	hook, err := pgStore.GetLastUpdated(context.Background(), "02c811e575be2df47d8b48dab3d3f1c9b0f6e16d0d40b5ed78253308fc2bd7170d")
	assert.NilError(t, err, "failed to get webhook from db")
	assert.Check(t, hook != nil, "hook should not be nil")
	assert.Equal(t, hook.Pubkey, "02c811e575be2df47d8b48dab3d3f1c9b0f6e16d0d40b5ed78253308fc2bd7170d", "pubkey should be 123")

	// Test that we are not able to attach the same lightning user for different pubkey.
	assert.NilError(t, pgStore.Set(context.Background(), Webhook{
		Pubkey: "02de1e98d0f87a1a5d9674f33d997b9c63cb65b27e10319cfa83b1b5ab58913f86",
		Url:    "http://example.com",
	}), "should not be able to use same url for different pubkey")

	// Test that we are able to update the same user registration for the same pubkey.
	assert.NilError(t,
		pgStore.Set(context.Background(), Webhook{
			Pubkey: "02c811e575be2df47d8b48dab3d3f1c9b0f6e16d0d40b5ed78253308fc2bd7170d",
			Url:    "http://example.com",
		}),
		"should be able to update the url for the same pubkey")

	assert.NilError(t, pgStore.DeleteExpired(context.Background(), time.Now()), "failed to delete expired")
}
