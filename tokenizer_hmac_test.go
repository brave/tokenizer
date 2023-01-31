package main

import (
	"errors"
	"testing"
)

func TestEmptyHMACKey(t *testing.T) {
	v := blob([]byte{1, 2, 3, 4})
	h := &hmacTokenizer{}
	_, err := h.tokenize(v)
	if !errors.Is(err, errNoKey) {
		t.Fatalf("Expected error '%v' but got '%v'.", errNoKey, err)
	}
}

func TestHMACPreservesLen(t *testing.T) {
	h := &hmacTokenizer{}
	if h.preservesLen() {
		t.Fatalf("HMAC tokenizer not expected to preserve length but it does.")
	}
}

func BenchmarkHMAC(b *testing.B) {
	h := hmacTokenizer{}
	_ = h.resetKey()
	v := blob([]byte{1, 2, 3, 4})

	for i := 0; i < b.N; i++ {
		_, _ = h.tokenize(v)
	}
}
