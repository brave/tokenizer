package main

import (
	"bytes"
	"testing"
	"time"

	uuid "github.com/google/uuid"
)

type dummyByter struct{}

func (d *dummyByter) bytes() []byte {
	return []byte("foobar")
}

func newV4(t *testing.T) uuid.UUID {
	u, err := uuid.NewRandom()
	if err != nil {
		t.Fatalf("Failed to get v4 UUID: %v", err)
	}
	return u
}

func TestStartStop(t *testing.T) {
	c := &config{
		keyExpiry:   time.Nanosecond,
		fwdInterval: time.Nanosecond,
	}
	for _, newAggregator := range ourAggregators {
		a := newAggregator()
		a.setConfig(c)
		a.use(newHmacTokenizer())
		a.connect(
			newStdinReceiver().inbox(),
			newStdoutForwarder().outbox(),
		)

		a.start()
		// Simply test if the function returns, i.e., there's no deadlock.
		time.Sleep(time.Millisecond)
		a.stop()
	}
}

func TestProcessBlob(t *testing.T) {
	inbox := make(chan serializer)
	outbox := make(chan token)
	tokenizer := newVerbatimTokenizer()
	_ = tokenizer.resetKey()
	c := &config{
		keyExpiry:   time.Second,
		fwdInterval: time.Second,
	}

	// Test the lifecycle of an aggregator and ensure that the data that goes
	// in is identical to the data coming out.
	for name, newAggregator := range ourAggregators {
		a := newAggregator()
		a.setConfig(c)
		a.use(tokenizer)
		a.connect(inbox, outbox)
		a.start()
		blob := &dummyByter{}
		inbox <- blob

		token := <-outbox
		if !bytes.Equal(blob.bytes(), token) {
			t.Fatalf("%s: Expected token %v but got %v.", name, blob.bytes(), token)
		}
		a.stop()
	}
}
