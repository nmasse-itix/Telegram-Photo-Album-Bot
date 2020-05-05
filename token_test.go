package main

import (
	"crypto"
	"encoding/hex"
	"testing"
	"time"

	"github.com/magiconair/properties/assert"
)

func TestTokenGenerator(t *testing.T) {
	// export KEY="$(openssl rand -hex 32)"
	secretHex := "6b68b32607bae2c3d5e140efd8f4d5b6518fced3081fc6b28478b903ceef9aa3"
	secret, err := hex.DecodeString(secretHex)
	if err != nil {
		t.Errorf("secretFromHex(): %s", err)
	}

	g, err := NewTokenGenerator(secret, crypto.SHA256)
	if err != nil {
		t.Errorf("NewTokenGenerator(): %s", err)
	}
	now := time.Unix(1588703522, 0) // date +%s
	token := g.NewToken(TokenData{now, "nmasse", "read"})

	// echo "000000: 021d 0000 6e6d 6173 7365 0072 6561 64" |xxd -r | openssl dgst -sha256 -mac HMAC -macopt "hexkey:$KEY" -binary |openssl base64
	expectedToken := "McChidYyEfEPkotTq08EW+eYHjd2QX+wlUzgGjOhWlY="
	assert.Equal(t, token, expectedToken, "expected a valid token")

	sixDaysLater := time.Unix(1589221922, 0)
	ok, err := g.ValidateToken(TokenData{sixDaysLater, "nmasse", "read"}, expectedToken, 7)
	if err != nil {
		t.Errorf("ValidateToken(sixDaysLater): %s", err)
	}
	if !ok {
		t.Errorf("ValidateToken(sixDaysLater): token is not valid")
	}

	sevenDaysLater := time.Unix(1589308322, 0)
	ok, err = g.ValidateToken(TokenData{sevenDaysLater, "nmasse", "read"}, expectedToken, 7)
	if err != nil {
		t.Errorf("ValidateToken(sevenDaysLater): %s", err)
	}
	if ok {
		t.Errorf("ValidateToken(sevenDaysLater): token is valid")
	}

}
