package main

// This file implements a flusher.  Callers can submit anonymized IP addresses
// to the flusher.  The flusher periodically POSTs all accumulated addresses to
// an HTTP-to-Kafka bridge that's running outside of the enclave.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	msg "github.com/brave-experiments/ia2/message"
)

// Flusher periodically flushes anonymized IP addresses to our HTTP-to-Kafka
// bridge.
type Flusher struct {
	sync.Mutex
	done          chan bool
	wg            sync.WaitGroup
	flushInterval time.Duration
	addrs         msg.WalletsByKeyID
	srvURL        string
}

// NewFlusher creates and returns a new Flusher.
func NewFlusher(flushInterval int, srvURL string) *Flusher {
	return &Flusher{
		flushInterval: time.Duration(flushInterval) * time.Second,
		addrs:         make(msg.WalletsByKeyID),
		done:          make(chan bool),
		srvURL:        srvURL,
	}
}

// Start starts the Flusher.
func (f *Flusher) Start() {
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		ticker := time.NewTicker(f.flushInterval)
		for {
			select {
			case <-f.done:
				return
			case <-ticker.C:
				l.Printf("Attempting to send %d anonymized addresses to Kafka bridge.", len(f.addrs))
				if err := f.sendBatch(); err != nil {
					l.Printf("Failed to send batch: %s", err)
				}
			}
		}
	}()
}

// sendBatch sends a batch of anonymized IP addresses to our Kafka bridge.
func (f *Flusher) sendBatch() error {
	f.Lock()
	defer f.Unlock()

	if len(f.addrs) == 0 {
		return nil
	}

	jsonBytes, err := json.Marshal(f.addrs)
	if err != nil {
		return fmt.Errorf("failed to marshal addresses: %s", err)
	}

	r := bytes.NewReader(jsonBytes)
	resp, err := http.Post(f.srvURL, "application/json", r)
	if err != nil {
		return fmt.Errorf("failed to post addresses to Kafka bridge: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("got HTTP code %d from Kafka bridge", resp.StatusCode)
	}

	l.Printf("Flushed %d addresses to Kafka bridge.", len(f.addrs))
	f.addrs = make(msg.WalletsByKeyID)

	return nil
}

// Stop stops the flusher.
func (f *Flusher) Stop() {
	f.done <- true
	f.wg.Wait()
	l.Println("Stopping flusher.")
}

// Submit submits the given anonymized IP address to the flusher.
func (f *Flusher) Submit(req *clientRequest) {
	f.Lock()
	defer f.Unlock()

	wallets, exists := f.addrs[req.KeyID]
	if !exists {
		// We're starting a new key ID epoch.
		wallets := make(msg.AddrsByWallet)
		wallets[req.Wallet] = msg.AddressSet{
			string(req.AnonAddr): msg.Empty{},
		}
		f.addrs[req.KeyID] = wallets
	} else {
		addrSet, exists := wallets[req.Wallet]
		if !exists {
			// We have no addresses for the given wallet yet.  Create a new
			// address set.
			wallets[req.Wallet] = msg.AddressSet{
				string(req.AnonAddr): msg.Empty{},
			}
		} else {
			// Add address to the given wallet's address set.
			addrSet[string(req.AnonAddr)] = msg.Empty{}
		}
	}
	l.Print(f.addrs)
}
