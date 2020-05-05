package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

type Secret []byte

func (r Secret) String() string {
	return hex.EncodeToString(r)
}

func (r Secret) Hashed() string {
	hash := sha256.Sum256(r)
	return hex.EncodeToString(hash[:])
}

func newRandomSecret(size int) (Secret, error) {
	var r Secret = make([]byte, size)
	_, err := rand.Read(r)
	if err != nil {
		return Secret{}, err
	}

	return r, nil
}

func secretFromHex(encoded string) (Secret, error) {
	return hex.DecodeString(encoded)
}
