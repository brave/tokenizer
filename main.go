package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	uuid "github.com/satori/go.uuid"
)

const (

	// We are unable to configure ia2 at runtime, which is why our
	// configuration options are constants.

	// useCryptoPAn uses Crypto-PAn anonymization instead of a HMAC.
	useCryptoPAn = true
	// flushInterval is the time interval after which we flush anonymized
	// addresses to our Kafka bridge.
	flushInterval = time.Minute * 5
	// kafkaBridgeURL points to a local socat listener that translates AF_INET
	// to AF_VSOCK.  In theory, we could talk directly to the AF_VSOCK address
	// of our Kafka bridge and get rid of socat but that makes testing more
	// annoying.  It easier to deal with tests via AF_INET.
	kafkaBridgeURL = "http://127.0.0.1:8081/addresses"
	// KeyExpiration determines the expiration time of the key that we use to
	// anonymize IP addresses.  Once the key expires, we rotate it by
	// generating a new one.
	KeyExpiration = time.Hour * 24 * 30 * 6
)

var (
	flusher    *Flusher
	anonymizer *Anonymizer
	l          = log.New(os.Stderr, "ia2: ", log.Ldate|log.Ltime|log.LUTC|log.Lshortfile)
	// Pre-defined UUID namespaces aren't a great fit for our use case, so we
	// use our own namespace, based on a randomly-generated V4 UUID.
	uuidNamespace = uuid.Must(uuid.FromString("c298cccd-3c75-4e72-a73b-47811ac13f4f"))
)

func main() {
	l.Printf("Running as UID %d.", os.Getuid())

	method := methodCryptoPAn
	if !useCryptoPAn {
		method = methodHMAC
	}
	anonymizer = NewAnonymizer(method, KeyExpiration)

	l.Printf("Initializing new flusher with interval %ds.", flushInterval)
	flusher = NewFlusher(flushInterval, kafkaBridgeURL)
	flusher.Start()
	defer flusher.Stop()

	r := chi.NewRouter()
	r.Post("/address", addressHandler)
	r.Get("/v1/confirmation/token/{walletID}", confTokenHandler)
	r.Get("/v2/confirmation/token/{walletID}", confTokenHandler)
	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}
	l.Fatal(srv.ListenAndServe())
}
