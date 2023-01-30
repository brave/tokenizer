package main

import (
	"sync"

	uuid "github.com/google/uuid"
)

// verbatimTokenizer implements a pseudo tokenizer that returns the same data
// that it was given.
type verbatimTokenizer struct {
	sync.RWMutex
	key *keyID
}

func newVerbatimTokenizer() tokenizer {
	return &verbatimTokenizer{}
}

func (v *verbatimTokenizer) tokenize(s serializer) (token, error) {
	v.RLock()
	defer v.RUnlock()

	if v.key == nil {
		return nil, errNoKey
	}
	return token(s.bytes()), nil
}

func (v *verbatimTokenizer) tokenizeAndKeyID(s serializer) (token, *keyID, error) {
	v.RLock()
	defer v.RUnlock()

	if v.key == nil {
		return nil, nil, errNoKey
	}
	return token(s.bytes()), v.key, nil
}

func (v *verbatimTokenizer) keyID() *keyID {
	v.RLock()
	defer v.RUnlock()

	return v.key
}

func (v *verbatimTokenizer) resetKey() error {
	v.Lock()
	defer v.Unlock()

	u, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	v.key = &keyID{UUID: u}
	return nil
}

func (v *verbatimTokenizer) preservesLen() bool {
	return true
}
