package main

// This file implements a flusher.  Callers can submit anonymized IP addresses
// to the flusher.  The flusher periodically POSTs all accumulated addresses to
// an HTTP-to-Kafka bridge that's running outside of the enclave.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	kutil "github.com/brave/ia2/kafkautils"
	msg "github.com/brave/ia2/message"
	"github.com/segmentio/kafka-go"
)

// Flusher periodically flushes anonymized IP addresses to our HTTP-to-Kafka
// bridge.
type Flusher struct {
	sync.Mutex
	done          chan bool
	wg            sync.WaitGroup
	flushInterval time.Duration
	addrs         msg.WalletsByKeyID
	writer        *kafka.Writer
	srvURL        string
}

// NewFlusher creates and returns a new Flusher.
func NewFlusher(flushInterval time.Duration, srvURL string) *Flusher {
	f := &Flusher{
		flushInterval: flushInterval,
		addrs:         make(msg.WalletsByKeyID),
		done:          make(chan bool),
		srvURL:        srvURL,
	}

	// If we're running outside an enclave, we can talk to Kafka directly,
	// without having to rely on our HTTP-to-Kafka bridge.  In that case,
	// instantiate a Kafka writer and don't use a bridge.
	kafkaWriter, err := kutil.NewKafkaWriter(
		kutil.DefaultKafkaCert,
		kutil.DefaultKafkaKey,
	)
	if err == nil {
		l.Println("Successfully instantiated Kafka writer; assuming we're outside an enclave.")
		f.writer = kafkaWriter
	} else {
		l.Printf("Not instantiating Kafka writer because: %s", err)
	}
	return f
}

// useKafkaDirectly returns true if we're supposed to talk to Kafka directly,
// instead of using our HTTP-to-Kafka bridge.
func (f *Flusher) useKafkaDirectly() bool {
	return f.writer != nil
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
				if err := f.sendBatch(); err != nil {
					l.Printf("Failed to send batch: %s", err)
				}
			}
		}
	}()
}

// sendBatch sends a batch of anonymized IP addresses to Kafka; either directly
// (if we're outside an enclave), or via our bridge (if we're in an enclave).
func (f *Flusher) sendBatch() error {
	f.Lock()
	defer f.Unlock()

	if len(f.addrs) == 0 {
		return nil
	}
	l.Printf("Attempting to send %d anonymized addresses to Kafka bridge.", len(f.addrs))

	jsonBytes, err := json.Marshal(f.addrs)
	if err != nil {
		return fmt.Errorf("failed to send anonymized addresses: %w", err)
	}

	if f.useKafkaDirectly() {
		l.Println("Attempting to send batch via Kafka.")
		err = f.sendBatchViaKafka(jsonBytes)
	} else {
		l.Println("Attempting to send batch via HTTP.")
		err = f.sendBatchViaHTTP(jsonBytes)
	}
	if err == nil {
		l.Println("Flushed addresses to back end.")
		f.addrs = make(msg.WalletsByKeyID)
	}
	return err
}

func (f *Flusher) sendBatchViaHTTP(jsonBytes []byte) error {
	r := bytes.NewReader(jsonBytes)
	resp, err := http.Post(f.srvURL, "application/json", r)
	if err != nil {
		return fmt.Errorf("failed to post addresses to Kafka bridge: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("got HTTP code %d from Kafka bridge", resp.StatusCode)
	}

	return nil
}

func (f *Flusher) sendBatchViaKafka(jsonBytes []byte) error {
	return f.writer.WriteMessages(context.Background(),
		kafka.Message{
			Key:   nil,
			Value: jsonBytes,
		},
	)
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
			fmt.Sprintf("%x", req.AnonAddr): msg.Empty{},
		}
		f.addrs[req.KeyID] = wallets
	} else {
		addrSet, exists := wallets[req.Wallet]
		if !exists {
			// We have no addresses for the given wallet yet.  Create a new
			// address set.
			wallets[req.Wallet] = msg.AddressSet{
				fmt.Sprintf("%x", req.AnonAddr): msg.Empty{},
			}
		} else {
			// Add address to the given wallet's address set.
			addrSet[fmt.Sprintf("%x", req.AnonAddr)] = msg.Empty{}
		}
	}
	l.Print(f.addrs)
}
