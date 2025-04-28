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
	testuser := "testuser"
	hook, err := pgStore.Set(context.Background(), Webhook{
		Pubkey:   "02c811e575be2df47d8b48dab3d3f1c9b0f6e16d0d40b5ed78253308fc2bd7170d",
		Url:      "http://example.com",
		Username: &testuser,
	})
	assert.NilError(t, err, "failed to set webhook")
	assert.Check(t, hook != nil, "hook should not be nil")
	assert.Equal(t, *hook.Username, "testuser", "username should be testuser")

	// Test that we are able to fetch the right webhook
	hook, err = pgStore.GetLastUpdated(context.Background(), "02c811e575be2df47d8b48dab3d3f1c9b0f6e16d0d40b5ed78253308fc2bd7170d")
	assert.NilError(t, err, "failed to get webhook from db")
	assert.Check(t, hook != nil, "hook should not be nil")
	assert.Equal(t, hook.Pubkey, "02c811e575be2df47d8b48dab3d3f1c9b0f6e16d0d40b5ed78253308fc2bd7170d", "pubkey should be 123")

	// Test that we are not able to attach the same lightning user for different pubkey.
	hook, err = pgStore.Set(context.Background(), Webhook{
		Pubkey: "02de1e98d0f87a1a5d9674f33d997b9c63cb65b27e10319cfa83b1b5ab58913f86",
		Url:    "http://example.com",
	})
	assert.NilError(t, err, "should not be able to use same url for different pubkey")
	assert.Check(t, hook != nil, "hook should not be nil")
	assert.Check(t, hook.Username == nil, "username should be nil")
	assert.Check(t, hook.Offer == nil, "offer should be nil")

	// Test that we are able to update the same user registration for the same pubkey.
	hook, err = pgStore.Set(context.Background(), Webhook{
		Pubkey: "02c811e575be2df47d8b48dab3d3f1c9b0f6e16d0d40b5ed78253308fc2bd7170d",
		Url:    "http://example.com",
	})
	assert.NilError(t, err, "should be able to update the url for the same pubkey")
	assert.Check(t, hook != nil, "hook should not be nil")
	assert.Check(t, hook.Username == nil, "username should be nil")
	assert.Check(t, hook.Offer == nil, "offer should be nil")

	// Test that we are able to update the same user registration with a different username.
	differenttestuser := "differenttestuser"
	hook, err = pgStore.Set(context.Background(), Webhook{
		Pubkey:   "02c811e575be2df47d8b48dab3d3f1c9b0f6e16d0d40b5ed78253308fc2bd7170d",
		Url:      "http://example.com",
		Username: &differenttestuser,
	})
	assert.NilError(t, err, "should be able to update the url for the same pubkey")
	assert.Check(t, hook != nil, "hook should not be nil")
	assert.Equal(t, *hook.Username, "differenttestuser", "username should be differenttestuser")
	assert.Check(t, hook.Offer == nil, "offer should be nil")

	// Test that we are not able to set the same username for different pubkey.
	hook, err = pgStore.Set(context.Background(), Webhook{
		Pubkey:   "02de1e98d0f87a1a5d9674f33d997b9c63cb65b27e10319cfa83b1b5ab58913f86",
		Url:      "http://example.com",
		Username: &differenttestuser,
	})
	assert.ErrorContains(t, err, "username conflict")
	assert.ErrorType(t, err, &ErrorUsernameConflict{})
	assert.Check(t, hook == nil, "hook should be nil")

	assert.NilError(t, pgStore.DeleteExpired(context.Background(), time.Now()), "failed to delete expired")

	// Test that we can set an offer for the same pubkey.
	offer := "lnoabcdefghijklmnopqrstuvwxyz1234567890"
	hook, err = pgStore.Set(context.Background(), Webhook{
		Pubkey:   "02c811e575be2df47d8b48dab3d3f1c9b0f6e16d0d40b5ed78253308fc2bd7170d",
		Url:      "http://example.com",
		Username: &differenttestuser,
		Offer:    &offer,
	})
	assert.NilError(t, err, "should be able to set an offer for the same pubkey")
	assert.Check(t, hook != nil, "hook should not be nil")
	assert.Equal(t, *hook.Username, "differenttestuser", "username should be differenttestuser")
	assert.Equal(t, *hook.Offer, "lnoabcdefghijklmnopqrstuvwxyz1234567890", "offer should be lnoabcdefghijklmnopqrstuvwxyz1234567890")

	// Test that we can set an offer for a new pubkey.
	offerusername := "offeruser"
	differentoffer := "lno1234567890abcdefghijklmnopqrstuvwxyz"
	hook, err = pgStore.Set(context.Background(), Webhook{
		Pubkey:   "03d749c8b0bec96c34b7e9243953b45e61abbc086acbdc9c9992c59c63e370d667",
		Url:      "http://example.com",
		Username: &offerusername,
		Offer:    &differentoffer,
	})
	assert.NilError(t, err, "should be able to set an offer for a new pubkey")
	assert.Check(t, hook != nil, "hook should not be nil")
	assert.Equal(t, *hook.Username, "offeruser", "username should be offeruser")
	assert.Equal(t, *hook.Offer, "lno1234567890abcdefghijklmnopqrstuvwxyz", "offer should be lno1234567890abcdefghijklmnopqrstuvwxyz")

	// Test that we can set update the offer for a new pubkey.
	updatedifferentoffer := "lno7890abcdefghijklmn123456opqrstuvwxyz"
	hook, err = pgStore.Set(context.Background(), Webhook{
		Pubkey:   "03d749c8b0bec96c34b7e9243953b45e61abbc086acbdc9c9992c59c63e370d667",
		Url:      "http://example.com",
		Username: &offerusername,
		Offer:    &updatedifferentoffer,
	})
	assert.NilError(t, err, "should be able to set an offer for a new pubkey")
	assert.Check(t, hook != nil, "hook should not be nil")
	assert.Equal(t, *hook.Username, "offeruser", "username should be offeruser")
	assert.Equal(t, *hook.Offer, "lno7890abcdefghijklmn123456opqrstuvwxyz", "offer should be lno7890abcdefghijklmn123456opqrstuvwxyz")
}

func TestPgStoreBolt12(t *testing.T) {
	pgStore, err := NewPgStore(os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("NewPgStore() error: %v", err)
	}

	assert.NilError(t, pgStore.DeleteExpired(context.Background(), time.Now()), "failed to delete expired")

	// Add a webhook for some pubkey
	testpubkey := "032c711e575be2df47d8b48dab3d3f1c9b0f6e16d0d40b5ed78253308fc2bd7170"
	testuser := "bolt12user"
	testoffer := "lno1234567890abcdefghijklmnopqrstuvwxyz"

	res, err := pgStore.SetPubkeyDetails(context.Background(), testpubkey, testuser, nil)
	assert.NilError(t, err, "failed to set")
	assert.Check(t, res != nil, "should not be nil")
	assert.Equal(t, res.Username, "bolt12user", "username should be bolt12user")

	// Test that we are able to fetch the right webhook
	res, err = pgStore.GetPubkeyDetails(context.Background(), "032c711e575be2df47d8b48dab3d3f1c9b0f6e16d0d40b5ed78253308fc2bd7170")
	assert.NilError(t, err, "failed to get from db")
	assert.Check(t, res != nil, "should not be nil")
	assert.Equal(t, res.Pubkey, "032c711e575be2df47d8b48dab3d3f1c9b0f6e16d0d40b5ed78253308fc2bd7170", "pubkey should be")

	// Test that we are not able to attach the same lightning user for different pubkey.
	differentpubkey := "042f3b9824e0ab9d68bee5a8321d439d5149069efaf787d309b21891cd7faa97d3"
	differentuser := "differentbolt12user"
	differentoffer := "lnoabcdefghijklmnopqrstuvwxyz1234567890"

	res, err = pgStore.SetPubkeyDetails(context.Background(), differentpubkey, testuser, &testoffer)
	assert.ErrorContains(t, err, "username conflict")
	assert.ErrorType(t, err, &ErrorUsernameConflict{})
	assert.Check(t, res == nil, "should be nil")

	// Test that we are able to update the same user registration for the same pubkey.
	res, err = pgStore.SetPubkeyDetails(context.Background(), testpubkey, testuser, &testoffer)
	assert.NilError(t, err, "should be able to update the same pubkey")
	assert.Check(t, res != nil, "should not be nil")
	assert.Equal(t, res.Username, "bolt12user", "username should be set")
	assert.Check(t, res.Offer != nil, "offer should be not nil")

	// Test that we are able to update the same user registration with a different username.
	res, err = pgStore.SetPubkeyDetails(context.Background(), testpubkey, differentuser, &testoffer)
	assert.NilError(t, err, "should be able to update the same pubkey")
	assert.Check(t, res != nil, "should not be nil")
	assert.Equal(t, res.Username, "differentbolt12user", "username should be differentbolt12user")
	assert.Check(t, res.Offer != nil, "offer should be not nil")

	// Test that we are not able to set the same username for different pubkey.
	thirdpubkey := "045a8c38c823b8648b9890361e3b1d0f0386975e0e11fd5fc9d64c9f8e8eaed0c0"

	res, err = pgStore.SetPubkeyDetails(context.Background(), thirdpubkey, differentuser, &testoffer)
	assert.ErrorContains(t, err, "username conflict")
	assert.ErrorType(t, err, &ErrorUsernameConflict{})
	assert.Check(t, res == nil, "hook should be nil")

	// Test that we are able to update the same user registration with a different offer.
	res, err = pgStore.SetPubkeyDetails(context.Background(), testpubkey, differentuser, &differentoffer)
	assert.NilError(t, err, "should be able to update the same pubkey")
	assert.Check(t, res != nil, "should not be nil")
	assert.Equal(t, res.Username, "differentbolt12user", "username should be differentbolt12user")
	assert.Equal(t, *res.Offer, "lnoabcdefghijklmnopqrstuvwxyz1234567890", "offer should be lnoabcdefghijklmnopqrstuvwxyz1234567890")
}
