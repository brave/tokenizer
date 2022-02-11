package main

import (
	"log"
	"net/http"
	"time"

	_ "github.com/brave-experiments/nitro-enclave-utils/randseed"

	nitro "github.com/brave-experiments/nitro-enclave-utils"
)

const (

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
	// KeyExpiration determines the expiration time of the key that we use to
	// anonymize IP addresses.  Once the key expires, we rotate it by
	// generating a new one.
	KeyExpiration = time.Hour * 24 * 30 * 6
)

var (
	flusher    *Flusher
	anonymizer *Anonymizer
	l          = log.New(os.Stderr, "ia2: ", log.Ldate|log.Ltime|log.LUTC|log.Lshortfile)
)

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
	enclave.AddRoute(http.MethodGet, "/v2/confirmation/token/{walletID}", confTokenHandler)

	method := methodCryptoPAn
	if !useCryptoPAn {
		method = methodHMAC
	}
	anonymizer = NewAnonymizer(method, KeyExpiration)

	// Start TCP proxy that translates AF_INET to AF_VSOCK, so that HTTP
	// requests that we make inside of ia2 can reach the SOCKS proxy that's
	// running on the parent EC2 instance.
	vproxy, err := NewVProxy()
	if err != nil {
		l.Fatalf("Failed to initialize vsock proxy: %s", err)
	}
	done := make(chan bool)
	go vproxy.Start(done)
	<-done

	l.Printf("Initializing new flusher with interval %ds.", flushInterval)
	flusher = NewFlusher(flushInterval, kafkaBridgeURL)
	flusher.Start()
	defer flusher.Stop()

	// Start blocks for as long as the enclave is alive.
	if err := enclave.Start(); err != nil {
		l.Fatalf("Enclave terminated: %v", err)
	}
}
