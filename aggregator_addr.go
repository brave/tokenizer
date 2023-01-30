package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	uuid "github.com/google/uuid"
	"github.com/linkedin/goavro/v2"
)

const (
	schemaService = "ADS"
	schemaSignal  = "ANON_IP_ADDRS"
)

// The Avro codec that we use to encode data before sending it to Kafka.
var ourCodec = func() *goavro.Codec {
	codec, err := goavro.NewCodec(`{
	"type": "record",
	"name": "DefaultMessage",
	"fields": [
		{ "name": "wallet_id", "type": "string" },
		{ "name": "service", "type": "string" },
		{ "name": "signal", "type": "string" },
		{ "name": "score", "type": "int" },
		{ "name": "justification", "type": "string" },
		{ "name": "created_at", "type": "string" }
	]}`)
	if err != nil {
		l.Fatalf("Failed to create Avro codec: %v", err)
	}
	return codec
}()

type kafkaMessage struct {
	WalletID      string `json:"wallet_id"`
	Service       string `json:"service"`
	Signal        string `json:"signal"`
	Score         int32  `json:"score"`
	Justification string `json:"justification"`
	CreatedAt     string `json:"created_at"`
}

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
		l.Fatalf("Failed to reset tokenizer key: %v", err)
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
					l.Fatalf("Failed to reset tokenizer key: %v", err)
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

// compileKafkaMsg turns the given arguments into a byte slice that's ready to
// be sent to our Kafka cluster.
func compileKafkaMsg(keyID keyID, walletID uuid.UUID, addrs AddressSet) ([]byte, error) {
	// We're abusing our schema's justification field by storing JSON in it.
	// While not elegant, this lets us ingest anonymized IP addresses without
	// modifying the schema.
	justification := struct {
		KeyID uuid.UUID `json:"keyid"`
		Addrs []string  `json:"addrs"`
	}{
		KeyID: keyID.UUID,
	}
	for addr := range addrs {
		justification.Addrs = append(justification.Addrs, addr)
	}
	jsonBytes, err := json.Marshal(justification)
	if err != nil {
		return nil, err
	}

	// Populate the remaining schema fields and turn it into JSON.
	msg := kafkaMessage{
		WalletID:      walletID.String(),
		Service:       schemaService,
		Signal:        schemaSignal,
		Justification: string(jsonBytes),
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	jsonBytes, err = json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Kafka message: %w", err)
	}

	return avroEncode(ourCodec, jsonBytes)
}

// flush flushes the aggregator's addresses to the outbox.
func (a *addrAggregator) flush() error {
	a.Lock()
	defer a.Unlock()

	if len(a.addrs) == 0 {
		return nil
	}

	for keyID, wallets := range a.addrs {
		// Compile the anonymized IP addresses that we've seen for a given
		// wallet ID.
		for walletID, addrSet := range wallets {
			kafkaMsg, err := compileKafkaMsg(keyID, walletID, addrSet)
			if err != nil {
				return err
			}
			a.outbox <- token(kafkaMsg)
		}
	}
	l.Printf("Forwarded %d address(es).", len(a.addrs))
	a.addrs = make(WalletsByKeyID)

	return nil
}
