package main

import (
	uuid "github.com/google/uuid"
)

// verbatimTokenizer implements a pseudo tokenizer that returns the same data
// that it was given.
type verbatimTokenizer struct {
	key *keyID
}

func newVerbatimTokenizer() tokenizer {
	return &verbatimTokenizer{}
}

func (v *verbatimTokenizer) tokenize(s serializer) (token, error) {
	if v.key == nil {
		return nil, errNoKey
	}
	return token(s.bytes()), nil
}

func (v *verbatimTokenizer) tokenizeAndKeyID(s serializer) (token, *keyID, error) {
	if v.key == nil {
		return nil, nil, errNoKey
	}
	return token(s.bytes()), v.key, nil
}

func (v *verbatimTokenizer) keyID() *keyID {
	return v.key
}

func (v *verbatimTokenizer) resetKey() error {
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
