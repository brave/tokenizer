package main

import (
	"testing"
	"time"
)

func TestReceiverStartStop(t *testing.T) {
	for _, newReceiver := range ourReceivers {
		r := newReceiver()
		r.start()
		r.stop()
	}
}

func TestReceiverSetConfig(t *testing.T) {
	for _, newReceiver := range ourReceivers {
		r := newReceiver()
		r.setConfig(&config{})
		r.setConfig(&config{port: uint16(1)})
		r.setConfig(&config{keyExpiry: time.Millisecond})
	}
}

func TestReceiverInbox(t *testing.T) {
	for _, newReceiver := range ourReceivers {
		r := newReceiver()
		_ = r.inbox()
	}
}
