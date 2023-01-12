package main

import (
	"errors"
	"testing"
)

func TestEmptyCryptoPAnKey(t *testing.T) {
	c := newCryptoPAnTokenizer()
	v := blob([]byte{1, 2, 3, 4})

	_, err := c.tokenize(v)
	if !errors.Is(err, errNoKey) {
		t.Fatalf("Expected error '%v' but got '%v'.", errNoKey, err)
	}
}

func TestCryptoPAnPreservesLen(t *testing.T) {
	c := newCryptoPAnTokenizer()
	if !c.preservesLen() {
		t.Fatalf("Crypto-PAn tokenizer expected to preserve length but it doesn't.")
	}
}

func TestBadBlobLen(t *testing.T) {
	c := newCryptoPAnTokenizer()
	v := blob([]byte{0})
	_ = c.resetKey()

	_, err := c.tokenize(v)
	if !errors.Is(err, errBadBlobLen) {
		t.Fatalf("Expected error '%v' but got '%v'.", errBadBlobLen, err)
	}

	_, _, err = c.tokenizeAndKeyID(v)
	if !errors.Is(err, errBadBlobLen) {
		t.Fatalf("Expected error '%v' but got '%v'.", errBadBlobLen, err)
	}
}

func BenchmarkCryptoPAn(b *testing.B) {
	c := newCryptoPAnTokenizer()
	_ = c.resetKey()
	v := blob([]byte{1, 2, 3, 4})

	for i := 0; i < b.N; i++ {
		_, _ = c.tokenize(v)
	}
}
