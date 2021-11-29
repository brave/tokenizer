package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

// Addresses represents a slice of anonymized IP addresses.
type Addresses struct {
	Addrs [][]byte `json:"addrs"`
}

// Flusher periodically flushes anonymized IP addresses to the given broker.
type Flusher struct {
	sync.Mutex
	done          chan bool
	wg            sync.WaitGroup
	flushInterval time.Duration
	addrs         Addresses
	broker        url.URL
	writer        *kafka.Writer
}

// NewFlusher creates and returns a new Flusher.
func NewFlusher(flushInterval int, broker url.URL, topic string) *Flusher {
	dialer := &kafka.Dialer{
		Timeout:   10 * time.Second,
		DualStack: true,
		TLS:       &tls.Config{},
	}

	return &Flusher{
		flushInterval: time.Duration(flushInterval) * time.Second,
		broker:        broker,
		writer: kafka.NewWriter(kafka.WriterConfig{
			Brokers:  []string{broker.String()},
			Topic:    topic,
			Balancer: &kafka.Hash{},
			Dialer:   dialer,
		}),
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

// sendBatch sends a batch of anonymized IP addresses to our backend.
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

	err = f.writer.WriteMessages(context.Background(),
		kafka.Message{
			Key:   nil,
			Value: []byte(jsonStr),
		},
	)
	if err != nil {
		log.Printf("Failed to write Kafka message: %s", err)
		return
	}

	log.Printf("Flushed %d addresses to Kafka broker.", len(f.addrs.Addrs))
	f.addrs.Addrs = nil
}

// Stop stops the flusher.
func (f *Flusher) Stop() {
	if err := f.writer.Close(); err != nil {
		log.Printf("Failed to close connection to Kafka broker: %s", err)
	}
	f.done <- true
	f.wg.Wait()
}

// Submit submits the given anonymized IP address to the flusher.
func (f *Flusher) Submit(req *clientRequest) {
	f.Lock()
	defer f.Unlock()
	f.addrs.Addrs = append(f.addrs.Addrs, req.AnonAddr)
}
