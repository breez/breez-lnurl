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
