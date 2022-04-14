package main

// This file implements an anonymizer that takes as input IP addresses (both v4
// and v6) and anonymizes them; either via HMAC or via Crypto-PAn.  The
// anonymizer's key eventually expire, after which is generates new keys.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Yawning/cryptopan"
	msg "github.com/brave-experiments/ia2/message"
	uuid "github.com/satori/go.uuid"
)

const (
	hmacKeySize     = 20
	methodCryptoPAn = iota
	methodHMAC
)

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
func (a *Anonymizer) Anonymize(addr net.IP) ([]byte, msg.KeyID) {
	a.Lock()
	var anonAddr []byte
	if a.method == methodHMAC {
		h := hmac.New(sha256.New, a.key)
		h.Write(addr)
		anonAddr = h.Sum(nil)
	} else if a.method == methodCryptoPAn {
		anonAddr = a.cryptoPAn.Anonymize(addr)
	}
	a.Unlock()

	return anonAddr, a.GetKeyID()
}

// GetKeyID returns the ID of the currently used anonymization key.  The key ID
// is the hex-encoded SHA-256 over the key.
func (a *Anonymizer) GetKeyID() msg.KeyID {
	a.Lock()
	defer a.Unlock()

	// A v5 UUID is supposed to hash the given name (in our case: the key)
	// using SHA-1 but let's be extra careful and hash the key using SHA-256
	// before handing it over to the uuid package.
	sum := sha256.Sum256(a.key)
	return msg.KeyID{UUID: uuid.NewV5(uuidNamespace, fmt.Sprintf("%x", sum[:]))}
}

// initKeys (re-)initializes the anonymization key.
func (a *Anonymizer) initKeys() {
	a.Lock()
	defer a.Unlock()

	var err error
	if a.method == methodHMAC {
		a.key = make([]byte, hmacKeySize)
		if _, err = rand.Read(a.key); err != nil {
			l.Fatal(err)
		}
		l.Println("Generated HMAC-SHA256 key for IP address anonymization.")
	} else if a.method == methodCryptoPAn {
		a.key = make([]byte, cryptopan.Size)
		if _, err = rand.Read(a.key); err != nil {
			l.Fatal(err)
		}
		if a.cryptoPAn, err = cryptopan.New(a.key); err != nil {
			l.Fatal(err)
		}
		l.Println("Generated Crypto-PAn key for IP address anonymization.")
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
	l.Println("Stopping anonymizer.")
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
