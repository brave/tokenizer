package main

// This file implements an anonymizer that takes as input IP addresses (both v4
// and v6) and anonymizes them; either via HMAC or via Crypto-PAn.  The
// anonymizer's key eventually expire, after which is generates new keys.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"log"
	"net"
	"sync"
	"time"

	"github.com/Yawning/cryptopan"
)

const (
	hmacKeySize     = 20
	methodCryptoPAn = iota
	methodHMAC
)

type keyID string

// Anonymizer implements an object that anonymizes IP addresses and
// periodically rotates the key that we use to anonymize addresses.
type Anonymizer struct {
	sync.Mutex
	method    int
	ticker    *time.Ticker
	done      chan bool
	key       []byte
	cryptoPAn *cryptopan.Cryptopan
}

// Anonymize takes as input an IP address and returns a byte slice that
// contains the anonymized IP address and the key ID that was used to anonymize
// the IP address.
func (a *Anonymizer) Anonymize(addr net.IP) ([]byte, keyID) {
	a.Lock()
	defer a.Unlock()

	var anonAddr []byte
	if a.method == methodHMAC {
		h := hmac.New(sha256.New, a.key)
		h.Write(addr)
		anonAddr = h.Sum(nil)
	} else if a.method == methodCryptoPAn {
		anonAddr = a.cryptoPAn.Anonymize(addr)
	}

	sum := sha256.Sum256(a.key)
	return anonAddr, keyID(sum[:])
}

// GetKeyID returns the ID of the currently used anonymization key.  The key ID
// is the SHA-256 over the key.
func (a *Anonymizer) GetKeyID() keyID {
	a.Lock()
	defer a.Unlock()

	sum := sha256.Sum256(a.key)
	return keyID(sum[:])
}

// initKeys (re-)initializes the anonymization key.
func (a *Anonymizer) initKeys() {
	a.Lock()
	defer a.Unlock()

	var err error
	if a.method == methodHMAC {
		a.key = make([]byte, hmacKeySize)
		if _, err = rand.Read(a.key); err != nil {
			log.Fatal(err)
		}
		log.Println("Generated HMAC-SHA256 key for IP address anonymization.")
	} else if a.method == methodCryptoPAn {
		a.key = make([]byte, cryptopan.Size)
		if _, err = rand.Read(a.key); err != nil {
			log.Fatal(err)
		}
		if a.cryptoPAn, err = cryptopan.New(a.key); err != nil {
			log.Fatal(err)
		}
		log.Println("Generated Crypto-PAn key for IP address anonymization.")
	}
}

// loop periodically re-initializes the anonymization key.
func (a *Anonymizer) loop() {
	defer a.ticker.Stop()

	for {
		select {
		case <-a.done:
			return
		case <-a.ticker.C:
			a.initKeys()
		}
	}
}

// Stop stops the anonymizer.
func (a *Anonymizer) Stop() {
	a.done <- true
}

// NewAnonymizer returns a new anonymizer using the given anonymization method
// and key expiration period.
func NewAnonymizer(method int, keyExpiration time.Duration) *Anonymizer {
	a := &Anonymizer{
		method: method,
		ticker: time.NewTicker(keyExpiration),
		done:   make(chan bool),
	}
	a.initKeys()
	go a.loop()

	return a
}
