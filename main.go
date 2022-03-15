package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	_ "github.com/brave-experiments/nitro-enclave-utils/randseed"
	"github.com/mdlayher/vsock"

	nitro "github.com/brave-experiments/nitro-enclave-utils"
	"github.com/brave-experiments/viproxy"
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
	// parentCID determines the CID (analogous to an IP address) of the parent
	// EC2 instance.  According to the AWS docs, it is always 3:
	// https://docs.aws.amazon.com/enclaves/latest/user/nitro-enclave-concepts.html
	parentCID = 3
	// parentProxyPort determines the TCP port of the SOCKS proxy that's
	// running on the parent EC2 instance.
	parentProxyPort = 1080
	// localProxy determines the IP address and port of the enclave-internal
	// proxy that translates between AF_INET and AF_VSOCK.
	localProxy = "127.0.0.1:1080"
)

var (
	flusher    *Flusher
	anonymizer *Anonymizer
	l          = log.New(os.Stderr, "ia2: ", log.Ldate|log.Ltime|log.LUTC|log.Lshortfile)
)

func main() {
	enclave := nitro.NewEnclave(
		&nitro.Config{
			SOCKSProxy: fmt.Sprintf("socks5://%s", localProxy),
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
	inAddr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:1080")
	if err != nil {
		l.Fatalf("Failed to resolve TCP address: %s", err)
	}
	tuple := &viproxy.Tuple{
		InAddr:  inAddr,
		OutAddr: &vsock.Addr{ContextID: uint32(parentCID), Port: uint32(parentProxyPort)},
	}
	proxy := viproxy.NewVIProxy([]*viproxy.Tuple{tuple})
	if err := proxy.Start(); err != nil {
		log.Fatalf("Failed to start VIProxy: %s", err)
	}

	l.Printf("Initializing new flusher with interval %ds.", flushInterval)
	flusher = NewFlusher(flushInterval, kafkaBridgeURL)
	flusher.Start()
	defer flusher.Stop()

	// Start blocks for as long as the enclave is alive.
	if err := enclave.Start(); err != nil {
		l.Fatalf("Enclave terminated: %v", err)
	}
}
