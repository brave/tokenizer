package main

import (
	"bytes"
	"errors"
	"testing"
)

var (
	// We're using 4-byte input because CryptoPAn only allows IP addresses
	// as input.
	value1 = blob([]byte{1, 2, 3, 4})
	value2 = blob([]byte{5, 6, 7, 8})
)

func TestTokenize(t *testing.T) {
	// Run the same tests over all our tokenizers.  This works as long as
	// there's a data format that they all accept.
	for name, newTokenizer := range ourTokenizers {
		tkzr := newTokenizer()

		if _, err := tkzr.tokenize(value1); !errors.Is(err, errNoKey) {
			t.Fatalf("%s: Expected error '%v' but got '%v'.", name, errNoKey, err)
		}

		_ = tkzr.resetKey()

		t1, err := tkzr.tokenize(value1)
		if err != nil {
			t.Fatalf("%s: Tokenize failed unexpectedly: %v", name, err)
		}
		t2, err := tkzr.tokenize(value1)
		if err != nil {
			t.Fatalf("%s: Tokenize failed unexpectedly: %v", name, err)
		}

		if !bytes.Equal(t1, t2) {
			t.Fatalf("%s: Tokenized values are not identical but they should be.", name)
		}

		t3, err := tkzr.tokenize(value2)
		if err != nil {
			t.Fatalf("%s: Tokenize failed unexpectedly: %v", name, err)
		}
		if bytes.Equal(t1, t3) {
			t.Fatalf("%s: Tokenized values are identical but they shouldn't be.", name)
		}
	}
}

func TestTokenizeAndKeyID(t *testing.T) {
	for name, newTokenizer := range ourTokenizers {
		tkzr := newTokenizer()

		_, _, err := tkzr.tokenizeAndKeyID(value1)
		if !errors.Is(err, errNoKey) {
			t.Fatalf("%s: Expected error '%v' but got '%v'.", name, errNoKey, err)
		}
		_ = tkzr.resetKey()

		token1, keyID1, err := tkzr.tokenizeAndKeyID(value1)
		if err != nil {
			t.Fatalf("%s: Unexpected error: %v", name, err)
		}

		token2, keyID2, err := tkzr.tokenizeAndKeyID(value1)
		if err != nil {
			t.Fatalf("%s: Unexpected error: %v", name, err)
		}

		if !bytes.Equal(token1, token2) {
			t.Fatalf("%s: Expected tokens to be identical but they aren't.", name)
		}
		if *keyID1 != *keyID2 {
			t.Fatalf("%s: Expected key IDs to be identical but they aren't.", name)
		}
	}
}

func TestKeyID(t *testing.T) {
	for name, newTokenizer := range ourTokenizers {
		tkzr := newTokenizer()
		if err := tkzr.resetKey(); err != nil {
			t.Fatalf("%s: Failed to reset keys: %v", name, err)
		}

		k1 := *tkzr.keyID()
		k2 := *tkzr.keyID()
		if k1 != k2 {
			t.Fatalf("%s: Expected key IDs to be equal but they aren't.", name)
		}

		_ = tkzr.resetKey()
		k3 := *tkzr.keyID()
		if k1 == k3 {
			t.Fatalf("%s: Expected different key IDs but they are identical.", name)
		}
	}
}

func TestResetKeys(t *testing.T) {
	var err error
	for name, newTokenizer := range ourTokenizers {
		tkzr := newTokenizer()
		_ = tkzr.resetKey()

		if _, err = tkzr.tokenize(value1); err != nil {
			t.Fatalf("%s: Failed to tokenize: %v", name, err)
		}

		if err = tkzr.resetKey(); err != nil {
			t.Fatalf("%s: Failed to reset keys: %v", name, err)
		}

		if _, err = tkzr.tokenize(value1); err != nil {
			t.Fatalf("%s: Failed to tokenize: %v", name, err)
		}

		// We're not testing if a key reset causes two identical blobs to map
		// to two different tokens because our verbatim tokenizer is
		// implemented as f(x) = x.
	}
}
