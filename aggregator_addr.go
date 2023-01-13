package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// addrAggregator implements an aggregator that keeps track of tokenized IP
// addresses and their respective meta data.
type addrAggregator struct {
	sync.RWMutex
	wg          sync.WaitGroup
	fwdInterval time.Duration
	keyExpiry   time.Duration
	addrs       WalletsByKeyID
	tokenizer   tokenizer
	inbox       chan serializer
	outbox      chan token
	done        chan empty
}

// newAddrAggregator returns a new address aggregator.
func newAddrAggregator() aggregator {
	return &addrAggregator{
		done:  make(chan empty),
		addrs: make(WalletsByKeyID),
	}
}

// setConfig sets the given configuration.
func (a *addrAggregator) setConfig(c *config) {
	a.Lock()
	defer a.Unlock()

	a.fwdInterval = c.fwdInterval
	a.keyExpiry = c.keyExpiry
	l.Printf("Forward interval: %s, key expiry: %s", a.fwdInterval, a.keyExpiry)
}

// use sets the tokenizer that must be used.
func (a *addrAggregator) use(t tokenizer) {
	a.Lock()
	defer a.Unlock()

	a.tokenizer = t
}

// connect sets the inbox to retrieve serialized data from and the outbox to
// send tokens to.
func (a *addrAggregator) connect(inbox chan serializer, outbox chan token) {
	a.Lock()
	defer a.Unlock()

	a.inbox = inbox
	a.outbox = outbox
}

// start starts the address aggregator.
func (a *addrAggregator) start() {
	if err := a.tokenizer.resetKey(); err != nil {
		l.Printf("Failed to reset key of tokenizer: %v", err)
	}
	a.wg.Add(1)

	go func() {
		defer a.wg.Done()
		a.RLock() // Protect read of fwdInterval and keyExpiry.
		fwdTicker := time.NewTicker(a.fwdInterval)
		keyTicker := time.NewTicker(a.keyExpiry)
		a.RUnlock()

		l.Println("Starting address aggregator loop.")
		for {
			select {
			case <-a.done:
				return
			case <-fwdTicker.C:
				if err := a.flush(); err != nil {
					l.Printf("Failed to forward addresses: %v", err)
				}
			case <-keyTicker.C:
				if err := a.tokenizer.resetKey(); err != nil {
					l.Printf("Failed to reset tokenizer key: %v", err)
				}
			case req := <-a.inbox:
				switch v := req.(type) {
				case *clientRequest:
					if err := a.processRequest(v); err != nil {
						l.Printf("Failed to process client request: %v", err)
					}
					l.Printf("Processed request for wallet %s.", v.Wallet)
				default:
					// We are not prepared to process whatever data structure
					// we were given.  Simply tokenize it and forward it right
					// away, without aggregation.
					t, err := a.tokenizer.tokenize(v)
					if err != nil {
						l.Printf("Failed to tokenize blob: %v", err)
					}
					a.outbox <- t
					l.Println("Type not supported.  Forwarded.")
				}
			}
		}
	}()
}

// stop stops the address aggregator.
func (a *addrAggregator) stop() {
	close(a.done)
	a.wg.Wait()
	l.Println("Stopped address aggregator.")
}

// processRequest processes an incoming client request.
func (a *addrAggregator) processRequest(req *clientRequest) error {
	a.Lock()
	defer a.Unlock()

	rawToken, keyID, err := a.tokenizer.tokenizeAndKeyID(req)
	if err != nil {
		return err
	}
	// The tokenized IP address may not be printable, so let's encode it.
	token := base64.StdEncoding.EncodeToString(rawToken)

	// If we're using a tokenizer that preserves the blob's length, we turn the
	// byte slice back into an IP address.
	if a.tokenizer.preservesLen() {
		if len(rawToken) != net.IPv4len && len(rawToken) != net.IPv6len {
			return errors.New("token is neither of length IPv4 nor IPv6")
		}
		token = net.IP(rawToken).String()
	}

	wallets, exists := a.addrs[*keyID]
	if !exists {
		// We're starting a new key ID epoch.
		wallets := make(AddrsByWallet)
		wallets[req.Wallet] = AddressSet{
			token: empty{},
		}
		a.addrs[*keyID] = wallets
	} else {
		// We're adding to the existing epoch.
		addrSet, exists := wallets[req.Wallet]
		if !exists {
			// We have no addresses for the given wallet yet.  Create a new
			// address set.
			wallets[req.Wallet] = AddressSet{
				token: empty{},
			}
		} else {
			// Add address to the given wallet's address set.
			addrSet[token] = empty{}
		}
	}
	return nil
}

// flush flushes the aggregator's addresses to the outbox.
func (a *addrAggregator) flush() error {
	a.Lock()
	defer a.Unlock()

	if len(a.addrs) == 0 {
		return nil
	}

	jsonBytes, err := json.Marshal(a.addrs)
	if err != nil {
		return fmt.Errorf("failed to marshal addresses: %w", err)
	}
	a.outbox <- token(jsonBytes)

	l.Printf("Forwarded %d addresses.", len(a.addrs))
	a.addrs = make(WalletsByKeyID)
	return nil
}
