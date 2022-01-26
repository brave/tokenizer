package main

import (
	"crypto/rand"
	"log"
	"net/http"

	_ "github.com/brave-experiments/nitro-enclave-utils/randseed"

	"github.com/Yawning/cryptopan"
	nitro "github.com/brave-experiments/nitro-enclave-utils"
)

const (
	hmacKeySize = 20

	// We are unable to configure ia2 at runtime, which is why our
	// configuration options are constants.

	// useCryptoPAn uses Crypto-PAn anonymization instead of a HMAC.
	useCryptoPAn = true
	// flushInterval is the time interval after which we flush anonymized
	// addresses to our Kafka bridge.
	flushInterval = 300
	// kafkaBridgeURL points to a local socat listener that translates AF_INET
	// to AF_VSOCK.  In theory, we could talk directly to the AF_VSOCK address
	// of our Kafka bridge and get rid of socat but that makes testing more
	// annoying.  It easier to deal with tests via AF_INET.
	kafkaBridgeURL = "http://127.0.0.1:8081"
)

var hmacKey []byte
var cryptoPAn *cryptopan.Cryptopan
var flusher *Flusher

// initAnonymization initializes the key material we need to anonymize IP
// addresses.
func initAnonymization(useCryptoPAn bool) {
	var err error
	if !useCryptoPAn {
		log.Println("Using HMAC-SHA256 for IP address anonymization.")
		hmacKey = make([]byte, hmacKeySize)
		if _, err = rand.Read(hmacKey); err != nil {
			log.Fatal(err)
		}
		log.Printf("HMAC key: %x", hmacKey)
	} else {
		log.Println("Using Crypto-PAn for IP address anonymization.")
		// Determine a cryptographically secure random number that serves as
		// key to our Crypto-PAn object.  The number is determined in the
		// enclave, and therefore unknown to us.
		buf := make([]byte, cryptopan.Size)
		if _, err = rand.Read(buf); err != nil {
			log.Fatal(err)
		}
		// In production mode, we are unable to see the enclave's debug output,
		// so there's no harm in logging secrets.
		log.Printf("Crypto-PAn key: %x", buf)
		cryptoPAn, err = cryptopan.New(buf)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func main() {
	enclave := nitro.NewEnclave(
		&nitro.Config{
			SOCKSProxy: "socks5://127.0.0.1:1080",
			FQDN:       "TODO",
			Port:       8080,
			UseACME:    false,
			Debug:      true,
		},
	)
	enclave.AddRoute(http.MethodPost, "/address", addressHandler)
	// The following endpoint must be identical to what our ads server exposes.
	enclave.AddRoute(http.MethodGet, "/v1/confirmation/token/{walletID}", confTokenHandler)

	initAnonymization(useCryptoPAn)

	// Start TCP proxy that translates AF_INET to AF_VSOCK, so that HTTP
	// requests that we make inside of ia2 can reach the SOCKS proxy that's
	// running on the parent EC2 instance.
	vproxy, err := NewVProxy()
	if err != nil {
		log.Fatalf("Failed to initialize vsock proxy: %s", err)
	}
	done := make(chan bool)
	go vproxy.Start(done)
	<-done

	log.Printf("Initializing new flusher with interval %ds.", flushInterval)
	flusher = NewFlusher(flushInterval, kafkaBridgeURL)
	flusher.Start()
	defer flusher.Stop()

	// Start blocks for as long as the enclave is alive.
	if err := enclave.Start(); err != nil {
		log.Fatalf("Enclave terminated: %v", err)
	}
}
