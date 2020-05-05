package main

import (
	"testing"

	"github.com/magiconair/properties/assert"
)

func TestRandomSecretLength(t *testing.T) {
	secret, err := newRandomSecret(32)
	if err != nil {
		t.Errorf("newRandomSecret(): %s", err)
	}
	assert.Equal(t, len(secret), 32, "random secret is 32 bytes long")
}

func TestSecretFromHex(t *testing.T) {
	secretHex := "11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff"
	secret, err := secretFromHex(secretHex)
	if err != nil {
		t.Errorf("secretFromHex(): %s", err)
	}
	assert.Equal(t, len(secret), 32, "secret value is 32 bytes long")
	assert.Equal(t, secret.String(), secretHex, "Secret.String prints the secret value as hex")
}

func TestSecretHashed(t *testing.T) {
	secretHex := "2e6cf592c0c41e57643b915dd719e0ffb681fd5183c3498e8a9802730a03c3e6"
	hashHex := "e4cb5359be709b6e35c48cfcfa2b661f576300000126dae2dd99d8949267c1c3"
	secret, err := secretFromHex(secretHex)
	if err != nil {
		t.Errorf("secretFromHex(): %s", err)
	}
	assert.Equal(t, secret.Hashed(), hashHex, "Secret.Hashed prints the hashed value as hex")
}
