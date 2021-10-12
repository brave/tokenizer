package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Addresses represents a slice of anonymized IP addresses.
type Addresses struct {
	Addrs [][]byte `json:"addrs"`
}

// Flusher periodically flushes anonymized IP addresses to the given backend.
type Flusher struct {
	sync.Mutex
	done          chan bool
	wg            sync.WaitGroup
	flushInterval time.Duration
	addrs         Addresses
	backend       url.URL
}

// NewFlusher creates and returns a new Flusher.
func NewFlusher(flushInterval int, backend url.URL) *Flusher {
	return &Flusher{
		flushInterval: time.Duration(flushInterval) * time.Second,
		backend:       backend,
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
				f.sendBatch()
			}
		}
	}()
}

func (f *Flusher) sendBatch() {
	f.Lock()
	defer f.Unlock()

	if len(f.addrs.Addrs) == 0 {
		return
	}

	jsonStr, err := json.Marshal(f.addrs)
	if err != nil {
		log.Printf("Failed to marshal addresses: %s", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, f.backend.String(), bytes.NewBuffer(jsonStr))
	if err != nil {
		log.Printf("Failed to create request: %s", err)
		return
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to send request: %s", err)
		return
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("Got HTTP status code %d from backend.", resp.StatusCode)
	}

	log.Printf("Flushed %d addresses to backend.", f.addrs.Addrs)
	f.addrs.Addrs = nil
}

// Stop stops the flusher.
func (f *Flusher) Stop() {
	f.done <- true
	f.wg.Wait()
}

// Submit submits the given anonymized IP address to the flusher.
func (f *Flusher) Submit(addr []byte) {
	f.Lock()
	defer f.Unlock()
	f.addrs.Addrs = append(f.addrs.Addrs, addr)
}
