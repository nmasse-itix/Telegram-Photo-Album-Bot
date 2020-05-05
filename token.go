package main

import (
	"bytes"
	"crypto"
	"crypto/hmac"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"time"
)

type TokenGenerator struct {
	AuthenticationKey []byte
	Algorithm         crypto.Hash
}

type TokenData struct {
	Timestamp   time.Time
	Username    string
	Entitlement string
}

func NewTokenGenerator(authenticationKey []byte, algorithm crypto.Hash) (*TokenGenerator, error) {
	if !algorithm.Available() {
		return nil, fmt.Errorf("Hash algorithm %d is not available", algorithm)
	}

	return &TokenGenerator{AuthenticationKey: authenticationKey, Algorithm: algorithm}, nil
}

func (g *TokenGenerator) NewToken(data TokenData) string {
	// Fill a buffer with the token data
	buffer := getBufferFor(data)

	// Pass the token data to the hash function
	hasher := hmac.New(g.Algorithm.New, g.AuthenticationKey)
	hasher.Write(buffer)
	hash := hasher.Sum(nil)

	//fmt.Println(hex.EncodeToString(hash))

	return base64.StdEncoding.EncodeToString(hash)
}

func getBufferFor(data TokenData) []byte {
	// Compute the number days since year 2000
	// Note: there is a one-off error if the token span across the end of a leap year
	var daysSinceY2K uint32 = uint32((data.Timestamp.Year()-2000)*365 + data.Timestamp.YearDay())
	//fmt.Printf("Days since Y2K = %d\n", daysSinceY2K)

	// Pack the token data in a buffer
	// - number of days since epoch
	// - username that generated the token
	// - entitlement for the resulting token
	usernameBytes := []byte(data.Username)
	entitlementBytes := []byte(data.Entitlement)
	bufferLen := len(usernameBytes) + len(entitlementBytes) + 5 // 4 bytes for daysSinceEpoch + one '\0' separator
	var buffer []byte = make([]byte, bufferLen)
	binary.LittleEndian.PutUint32(buffer, daysSinceY2K)
	start, stop := 4, 4+len(usernameBytes)
	copy(buffer[start:stop], usernameBytes)
	start, stop = stop+1, stop+1+len(entitlementBytes)
	copy(buffer[start:stop], entitlementBytes)

	//fmt.Println(hex.EncodeToString(buffer))

	return buffer
}

func (g *TokenGenerator) ValidateToken(data TokenData, token string, validity int) (bool, error) {
	rawToken, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return false, err
	}

	hasher := hmac.New(g.Algorithm.New, g.AuthenticationKey)
	for days := 0; days < validity; days = days + 1 {
		attempt := data
		attempt.Timestamp = data.Timestamp.Add(time.Hour * -24 * time.Duration(days))
		buffer := getBufferFor(attempt)
		hasher.Reset()
		hasher.Write(buffer)
		hash := hasher.Sum(nil)
		if bytes.Compare(hash, rawToken) == 0 {
			return true, nil
		}
	}

	return false, nil
}
