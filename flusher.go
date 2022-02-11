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

// addresses maps a wallet ID to a set of its anonymized IP addresses, all
// represented as strings.
type addresses map[uuid.UUID]addressSet

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
func (a addresses) MarshalJSON() ([]byte, error) {
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

// Flusher periodically flushes anonymized IP addresses to our HTTP-to-Kafka
// bridge.
type Flusher struct {
	sync.Mutex
	done          chan bool
	wg            sync.WaitGroup
	flushInterval time.Duration
	addrs         addresses
	srvURL        string
}

// NewFlusher creates and returns a new Flusher.
func NewFlusher(flushInterval int, srvURL string) *Flusher {
	return &Flusher{
		flushInterval: time.Duration(flushInterval) * time.Second,
		addrs:         make(addresses),
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
	f.addrs = make(addresses)

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

	_, exists := f.addrs[req.Wallet]
	if !exists {
		addrsSet := make(addressSet)
		addrsSet[string(req.AnonAddr)] = empty{}
		f.addrs[req.Wallet] = addrsSet
	} else {
		f.addrs[req.Wallet][string(req.AnonAddr)] = empty{}
	}
	l.Print(f.addrs)
}
