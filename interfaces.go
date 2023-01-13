package main

// Tokenizer has four conceptual components. The following diagram illustrates
// how these components interact.  The receiver receives incoming data and
// forwards it to the aggregator, which takes advantage of the tokenizer to
// turn data into tokens, which are then sent to the forwarder.
//
// ┏━━━━━━━━━━┓    ┏━━━━━━━━━━━━┓    ┏━━━━━━━━━━━┓
// ┃ Receiver ┃ ━> ┃ Aggregator ┃ ━> ┃ Forwarder ┃
// ┗━━━━━━━━━━┛    ┗━━━━━━━━━━━━┛    ┗━━━━━━━━━━━┛
//                       ┃
//                       v
//                 ┏━━━━━━━━━━━┓
//                 ┃ Tokenizer ┃
//                 ┗━━━━━━━━━━━┛

import (
	"time"

	uuid "github.com/google/uuid"
)

type blob []byte
type token []byte

func (b blob) bytes() []byte {
	return b
}

// config stores the configuration for each component, all in one data
// structure.  Considering that we have few and simple components for now,
// that's acceptable.
type config struct {
	kafkaConfig *kafkaConfig
	fwdInterval time.Duration
	keyExpiry   time.Duration
	port        uint16
}

type components struct {
	r receiver
	a aggregator
	t tokenizer
	f forwarder
}

type keyID struct {
	uuid.UUID
}

// serializer allows for serializing data into a byte slice.
type serializer interface {
	bytes() []byte
}

// configurer allows for setting the configuration.
type configurer interface {
	setConfig(*config)
}

// startStopper allows for starting and stopping.
type startStopper interface {
	start()
	stop()
}

// receiver receives input data from somewhere.  The data can be of arbitrary
// nature and come from anywhere as long as it supports the serializer
// interface.
type receiver interface {
	inbox() chan serializer
	startStopper
	configurer
}

// aggregator aggregates and manages data by sitting in between the receiver
// and forwarder while using the tokenizer.
type aggregator interface {
	connect(inbox chan serializer, outbox chan token)
	use(tokenizer)
	startStopper
	configurer
}

// tokenizer turns a serializer object into tokens, which typically involves a
// secret key.
type tokenizer interface {
	keyID() *keyID
	tokenize(serializer) (token, error)
	tokenizeAndKeyID(serializer) (token, *keyID, error)
	resetKey() error
	preservesLen() bool
}

// forwarder sends tokens somewhere.  Anywhere, really.
type forwarder interface {
	outbox() chan token
	startStopper
	configurer
}
