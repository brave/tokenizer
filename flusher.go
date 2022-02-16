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

	uuid "github.com/satori/go.uuid"
)

// empty represents an empty map value.
type empty struct{}

// addressSet represents a set of string-encoded IP addresses.
type addressSet map[string]empty

// addrsByWallet maps a wallet ID to a set of its anonymized IP addresses, all
// represented as strings.
type addrsByWallet map[uuid.UUID]addressSet

// walletsByKeyID maps a key ID to a map of type addrsByWallet.  Key IDs
// represent data collection epochs: whenever the key ID rotates, a new epoch
// begins, and our collection of wallet-to-address records begins afresh.
type walletsByKeyID map[KeyID]addrsByWallet

// MarshalJSON marshals the given key ID-to-wallets map and turns it into the
// following JSON:
//
// {
//   "keyid": {
//     "foo": {
//       "addrs": {
//         "68a7deb0-615c-4f26-bf87-6b122732d8e9": [
//           "1.1.1.1",
//           "2.2.2.2",
//           ...
//         ],
//         ...
//       }
//     }
//   }
// }
func (w walletsByKeyID) MarshalJSON() ([]byte, error) {
	type toMarshal struct {
		WalletsByKeyID map[KeyID]addrsByWallet `json:"keyid"`
	}
	m := &toMarshal{WalletsByKeyID: make(walletsByKeyID)}
	for keyID, wallets := range w {
		m.WalletsByKeyID[keyID] = wallets
	}
	return json.Marshal(m)
}

// MarshalJSON marshals the given addresses and turns it into the following
// JSON:
//
// {
//   "addrs": {
//     "68a7deb0-615c-4f26-bf87-6b122732d8e9": [
//       "1.1.1.1",
//       "2.2.2.2",
//       ...
//     ],
//     ...
//   }
// }
func (a addrsByWallet) MarshalJSON() ([]byte, error) {
	type toMarshal struct {
		Addrs map[string][]string `json:"addrs"`
	}
	m := &toMarshal{Addrs: make(map[string][]string)}
	for wallet, addrSet := range a {
		addrSlice := []string{}
		for addr := range addrSet {
			addrSlice = append(addrSlice, addr)
		}
		m.Addrs[wallet.String()] = addrSlice
	}
	return json.Marshal(m)
}

// String implements the Stringer interface for addrsByWallet.
func (a addrsByWallet) String() string {
	allAddrSet := make(map[string]empty)
	for _, addrSet := range a {
		for a := range addrSet {
			allAddrSet[a] = empty{}
		}

	}
	return fmt.Sprintf("Holding %d wallet addresses containing a total of %d unique IP addresses.", len(a), len(allAddrSet))
}

// Flusher periodically flushes anonymized IP addresses to our HTTP-to-Kafka
// bridge.
type Flusher struct {
	sync.Mutex
	done          chan bool
	wg            sync.WaitGroup
	flushInterval time.Duration
	addrs         walletsByKeyID
	srvURL        string
}

// NewFlusher creates and returns a new Flusher.
func NewFlusher(flushInterval int, srvURL string) *Flusher {
	return &Flusher{
		flushInterval: time.Duration(flushInterval) * time.Second,
		addrs:         make(walletsByKeyID),
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
	f.addrs = make(walletsByKeyID)

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
		wallets := make(addrsByWallet)
		wallets[req.Wallet] = addressSet{
			string(req.AnonAddr): empty{},
		}
		f.addrs[req.KeyID] = wallets
	} else {
		addrSet, exists := wallets[req.Wallet]
		if !exists {
			// We have no addresses for the given wallet yet.  Create a new
			// address set.
			wallets[req.Wallet] = addressSet{
				string(req.AnonAddr): empty{},
			}
		} else {
			// Add address to the given wallet's address set.
			addrSet[string(req.AnonAddr)] = empty{}
		}
	}
	l.Print(f.addrs)
}
