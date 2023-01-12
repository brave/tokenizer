package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	uuid "github.com/satori/go.uuid"
)

const (
	hmacKeySize = 20 // In bytes.
)

// hmacTokenizer implements a tokenizer that uses HMAC-SHA256.
type hmacTokenizer struct {
	key []byte
}

func newHmacTokenizer() tokenizer {
	return &hmacTokenizer{}
}

func (h *hmacTokenizer) tokenize(s serializer) (token, error) {
	if len(h.key) == 0 {
		return nil, errNoKey
	}
	t := hmac.New(sha256.New, h.key)
	t.Write(s.bytes())
	return t.Sum(nil), nil
}

func (h *hmacTokenizer) tokenizeAndKeyID(s serializer) (token, *keyID, error) {
	if len(h.key) == 0 {
		return nil, nil, errNoKey
	}
	t := hmac.New(sha256.New, h.key)
	t.Write(s.bytes())
	return t.Sum(nil), h.keyID(), nil
}

func (h *hmacTokenizer) keyID() *keyID {
	// A v5 UUID is supposed to hash the given name (in our case: the key)
	// using SHA-1 but let's be extra careful and hash the key using SHA-256
	// before handing it over to the uuid package.
	sum := sha256.Sum256(h.key)
	return &keyID{UUID: uuid.NewV5(uuidNamespace, fmt.Sprintf("%x", sum[:]))}
}

func (h *hmacTokenizer) resetKey() error {
	h.key = make([]byte, hmacKeySize)
	_, err := rand.Read(h.key)
	return err
}

func (h *hmacTokenizer) preservesLen() bool {
	return false
}
