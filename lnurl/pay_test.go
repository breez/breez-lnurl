package lnurl

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/breez/lspd/lightning"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/tv42/zbase32"
	"gotest.tools/assert"
)

func TestPayRegisterLnurlPayRequestValidUsername(t *testing.T) {
	domain := "lnurl.domain"
	url := fmt.Sprintf("http://%v/callback", domain)
	time := time.Now().Unix()
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Errorf("failed to generate private key %v", err)
	}
	pubkey := privKey.PubKey()
	serializedPubkey := hex.EncodeToString(pubkey.SerializeCompressed())

	// Test valid usernames
	validUsernames := []string{
		"testuser",
		"test.user",
		"test#user",
		"test{user}",
		"test+user",
		"this________username________is________not________too________long",
	}

	for _, validUsername := range validUsernames {
		messgeToSign := fmt.Sprintf("%v-%v-%v", time, url, validUsername)
		msg := append(lightning.SignedMsgPrefix, []byte(messgeToSign)...)
		first := sha256.Sum256([]byte(msg))
		second := sha256.Sum256(first[:])
		sig, err := ecdsa.SignCompact(privKey, second[:], true)
		if err != nil {
			t.Errorf("failed to sign signature %v", err)
		}
		payRequest := RegisterLnurlPayRequest{
			Username:   &validUsername,
			Time:       time,
			WebhookUrl: url,
			Signature:  zbase32.EncodeToString(sig),
		}
		log.Printf("username: %v", *payRequest.Username)
		err = payRequest.Verify(serializedPubkey)
		assert.NilError(t, err, "should be able a valid username")
	}
}

func TestPayRegisterLnurlPayRequestInvalidUsername(t *testing.T) {
	domain := "lnurl.domain"
	url := fmt.Sprintf("http://%v/callback", domain)
	time := time.Now().Unix()
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Errorf("failed to generate private key %v", err)
	}
	pubkey := privKey.PubKey()
	serializedPubkey := hex.EncodeToString(pubkey.SerializeCompressed())

	// Test invalid usernames
	invalidUsernames := []string{
		"testuser.",
		".testuser",
		"test..user",
		"test(user",
		"test≠user",
		"this___________username___________is___________too___________long",
	}

	for _, invalidUsername := range invalidUsernames {
		messgeToSign := fmt.Sprintf("%v-%v-%v", time, url, invalidUsername)
		msg := append(lightning.SignedMsgPrefix, []byte(messgeToSign)...)
		first := sha256.Sum256([]byte(msg))
		second := sha256.Sum256(first[:])
		sig, err := ecdsa.SignCompact(privKey, second[:], true)
		if err != nil {
			t.Errorf("failed to sign signature %v", err)
		}
		payRequest := RegisterLnurlPayRequest{
			Username:   &invalidUsername,
			Time:       time,
			WebhookUrl: url,
			Signature:  zbase32.EncodeToString(sig),
		}
		log.Printf("username: %v", *payRequest.Username)
		err = payRequest.Verify(serializedPubkey)
		assert.ErrorContains(t, err, "invalid username")
	}
}

func TestPayRegisterLnurlPayRequestValidOffers(t *testing.T) {
	domain := "lnurl.domain"
	url := fmt.Sprintf("http://%v/callback", domain)
	time := time.Now().Unix()
	username := "testuser"
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Errorf("failed to generate private key %v", err)
	}
	pubkey := privKey.PubKey()
	serializedPubkey := hex.EncodeToString(pubkey.SerializeCompressed())

	// Test valid offers
	validOffers := []string{
		"lno1zzfq9ktw4h4r67qpq3zf4jjujdrpeenuz4jw9cwhxgjl5e7a8wvh5cqcqvet65ahjawgr0r0uk0xznn0d5hrlpn2pqkqpeauwd4lxn33kjha7qgz4g9uzme8aakpehdzgel76lne3sswk6ducu6ygnsh8d87fqah39psqtqweqrf5actfuucvmmlt3k6snksj9dhsgvscj3aa2prf3p386q7p9kzhek7n0aspfmzxpps793pq0kufnlevx9qtyem0tq5g5lym8xt6zcve2kgqe5wv3gf9fcqkmt2z",
	}

	for _, offer := range validOffers {
		messgeToSign := fmt.Sprintf("%v-%v-%v-%v", time, url, username, offer)
		msg := append(lightning.SignedMsgPrefix, []byte(messgeToSign)...)
		first := sha256.Sum256([]byte(msg))
		second := sha256.Sum256(first[:])
		sig, err := ecdsa.SignCompact(privKey, second[:], true)
		if err != nil {
			t.Errorf("failed to sign signature %v", err)
		}
		payRequest := RegisterLnurlPayRequest{
			Time:       time,
			WebhookUrl: url,
			Username:   &username,
			Offer:      &offer,
			Signature:  zbase32.EncodeToString(sig),
		}
		log.Printf("offer: %v", *payRequest.Offer)
		err = payRequest.Verify(serializedPubkey)
		assert.NilError(t, err, "should be able a valid offer")
	}
}

func TestPayRegisterLnurlPayRequestInvalidOffers(t *testing.T) {
	domain := "lnurl.domain"
	url := fmt.Sprintf("http://%v/callback", domain)
	time := time.Now().Unix()
	username := "testuser"
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Errorf("failed to generate private key %v", err)
	}
	pubkey := privKey.PubKey()
	serializedPubkey := hex.EncodeToString(pubkey.SerializeCompressed())

	// Test valid offers
	validOffers := []string{
		"thisisnotavalidoffer",
		"LNO1ZZFQ9KTW4H4R67QPQ3ZF4JJUJDRPEENUZ4JW9CWHXGJL5E7A8WVH5CQCQVET65AHJAWGR0R0UK0XZN0D5HRLPN2PQKQPEAUWD4LXN33KJHA7QGZ4G9UZME8AAKPEHDZGEL76LNE3SSWK6DUCU6YGNSH8D87FQAH39PSQTQWEQRRF5ACTFUUCVMMMLT3K6SNKSJ9DHSGVSCJ3AA2PRF3P386Q7P9KZHEK7N0ASPFMZXPPS793PQ0KUFNLEVX9QTYYEM0TQ5G5LYM8XT6ZCVE2KGQE5WV3GF9FCQKMT2Z",
		"LNINVALIDOFFER",
	}

	for _, offer := range validOffers {
		messgeToSign := fmt.Sprintf("%v-%v-%v-%v", time, url, username, offer)
		msg := append(lightning.SignedMsgPrefix, []byte(messgeToSign)...)
		first := sha256.Sum256([]byte(msg))
		second := sha256.Sum256(first[:])
		sig, err := ecdsa.SignCompact(privKey, second[:], true)
		if err != nil {
			t.Errorf("failed to sign signature %v", err)
		}
		payRequest := RegisterLnurlPayRequest{
			Time:       time,
			WebhookUrl: url,
			Username:   &username,
			Offer:      &offer,
			Signature:  zbase32.EncodeToString(sig),
		}
		log.Printf("offer: %v", *payRequest.Offer)
		err = payRequest.Verify(serializedPubkey)
		assert.ErrorContains(t, err, "invalid offer")
	}
}
