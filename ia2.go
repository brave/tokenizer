package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/Yawning/cryptopan"
	_ "github.com/brave-experiments/ia2/init_rand"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mdlayher/vsock"
	"github.com/milosgajdos/tenus"
	"github.com/paulbellamy/ratecounter"
	uuid "github.com/satori/go.uuid"

	"golang.org/x/crypto/acme/autocert"
)

const (
	acmeCertCacheDir  = "cert-cache"
	hmacKeySize       = 20
	entropySeedDevice = "/dev/random"
	entropySeedSize   = 2048
	nonceSize         = 40 // The number of hex digits in a nonce.

	// We are unable to configure ia2 at runtime, which is why our
	// configuration options are constants.
	useAcme       = false  // Use ACME to obtain certificates.
	debug         = true   // Enable debug mode, which logs extra information.
	useCryptoPAn  = true   // Use Crypto-PAn anonymization instead of a HMAC.
	fqdn          = "TODO" // FQDN for TLS certificate.
	broker        = "TODO" // Kafka broker URL to send anonymized IP addresses to.
	topic         = "TODO" // Kafka topic.
	srvPort       = 8080   // Port that our HTTPS server is listening on.
	flushInterval = 300    // Time interval after which we flush addresses to the broker.
)

var certSha256 string
var hmacKey []byte
var cryptoPAn *cryptopan.Cryptopan
var counter = ratecounter.NewRateCounter(1 * time.Second)
var flusher *Flusher
var nonceRegExp = fmt.Sprintf("[a-f0-9]{%d}", nonceSize)

// clientRequest represents a client's confirmation token request.  It contains
// the client's IP address, wallet ID, and eventually its anonymized IP
// address.
type clientRequest struct {
	Addr     net.IP
	AnonAddr []byte
	Wallet   uuid.UUID
}

// isValidRequest returns true if the given request is POST and its form data
// could be successfully parsed.
func isValidRequest(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "got %s but expected %s request\n", r.Method, http.MethodPost)
		return false
	}
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "failed to parse %s data: %v\n", http.MethodPost, err)
		return false
	}
	return true
}

// setupAcme attempts to retrieve an HTTPS certificate from Let's Encrypt for
// the given FQDN.  Note that we are unable to cache certificates across
// enclave restarts, so the enclave requests a new certificate each time it
// starts.  If the restarts happen often, we may get blocked by Let's Encrypt's
// rate limiter for a while.
func setupAcme(fqdn string, server *http.Server) {
	var err error

	log.Printf("ACME hostname set to %s.", fqdn)
	var cache autocert.Cache
	if err = os.MkdirAll(acmeCertCacheDir, 0700); err != nil {
		log.Fatalf("Failed to create cache directory: %v", err)
	} else {
		cache = autocert.DirCache(acmeCertCacheDir)
	}
	certManager := autocert.Manager{
		Cache:      cache,
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist([]string{fqdn}...),
	}
	go func() {
		// Let's Encrypt's HTTP-01 challenge requires a listener on port 80:
		// https://letsencrypt.org/docs/challenge-types/#http-01-challenge
		l, err := vsock.Listen(uint32(80))
		if err != nil {
			log.Fatalf("Failed to listen for HTTP-01 challenge: %s", err)
		}
		defer func() {
			_ = l.Close()
		}()

		log.Printf("Starting autocert listener.")
		_ = http.Serve(l, certManager.HTTPHandler(nil))
	}()
	server.TLSConfig = &tls.Config{GetCertificate: certManager.GetCertificate}

	go func() {
		// Wait until the HTTP-01 listener returned and then check if our new
		// certificate is cached.
		var rawData []byte
		for {
			// Get the SHA-1 hash over our leaf certificate.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			rawData, err = cache.Get(ctx, fqdn)
			if err != nil {
				time.Sleep(5 * time.Second)
			} else {
				log.Printf("Got certificates from cache.  Proceeding with start.")
				break
			}
		}
		setCertFingerprint(rawData)
	}()
}

// setCertFingerprint takes as input a PEM-encoded certificate and extracts its
// SHA-256 fingerprint.  We need the certificate's fingerprint because we embed
// it in attestation documents, to bind the enclave's certificate to the
// attestation document.
func setCertFingerprint(rawData []byte) {
	rest := []byte{}
	for rest != nil {
		block, rest := pem.Decode(rawData)
		if block == nil {
			log.Fatal("pem.Decode failed because it didn't find PEM data in the input we provided.")
		}
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				log.Fatalf("Failed to parse X509 certificate: %v", err)
			}
			if !cert.IsCA {
				certSha256 = fmt.Sprintf("%x", sha256.Sum256(cert.Raw))
				log.Printf("SHA-256 of server's certificate: %s", certSha256)
				return
			}
		}
		rawData = rest
	}
}

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

// assignLoAddr assigns an IP address to the loopback interface, which is
// necessary because Nitro enclaves don't do that out-of-the-box.  We need the
// loopback interface because we run a simple TCP proxy that listens on
// 127.0.0.1:1080 and converts AF_INET to AF_VSOCK.
func assignLoAddr() error {
	addrStr := "127.0.0.1/8"
	l, err := tenus.NewLinkFrom("lo")
	if err != nil {
		return err
	}
	addr, network, err := net.ParseCIDR(addrStr)
	if err != nil {
		return err
	}
	if err = l.SetLinkIp(addr, network); err != nil {
		return err
	}
	if err = l.SetLinkUp(); err != nil {
		return err
	}
	log.Printf("Assigned %s to loopback interface.", addrStr)
	return nil
}

// setEnvVar sets an environment variable identified by key to value.
func setEnvVar(key, value string) {
	if err := os.Setenv(key, value); err != nil {
		// If we are unable to set our environment variables, ia2 won't be able
		// to send messages to our Kafka broker, so we might as well fail out.
		log.Fatalf("Failed to set %q to %q: %s", key, value, err)
	}
}

func main() {
	var err error

	ignoreNitro := flag.Bool("local", false, "Skip Nitro-specific code, to facilitate debugging.")
	flag.Parse()

	if debug {
		log.Println("Enabling debug mode.")
		ticker := time.NewTicker(1 * time.Second)
		go func() {
			for range ticker.C {
				if rate := counter.Rate(); rate > 0 {
					log.Printf("Submit requests per second: %d", rate)
				}
			}
		}()
	}

	if !*ignoreNitro {
		if err = assignLoAddr(); err != nil {
			log.Fatalf("Failed to assign address to lo: %s", err)
		}
	}

	log.Println("Setting up HTTP handlers.")
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Get("/attest", attestationHandler)
	router.Get("/submit", submitHandler)
	// The following endpoint must be identical to what our ads server exposes.
	router.Get("/v1/confirmation/token/{walletID}", confTokenHandler)

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
	setEnvVar("HTTP_PROXY", "socks5://127.0.0.1:1080")
	setEnvVar("HTTPS_PROXY", "socks5://127.0.0.1:1080")

	log.Printf("Initializing new flusher with interval %ds and broker %s.", flushInterval, broker)
	brokerURL, err := url.Parse(broker)
	if err != nil {
		log.Fatal(err)
	}
	flusher = NewFlusher(flushInterval, *brokerURL, topic)
	flusher.Start()
	defer flusher.Stop()

	server := http.Server{
		Addr:    fmt.Sprintf(":%d", srvPort),
		Handler: router,
	}
	if useAcme {
		setupAcme(fqdn, &server)
	} else {
		cert, err := genSelfSignedCert(fqdn)
		if err != nil {
			log.Fatalf("Failed to generate self-signed certificate: %v", err)
		}
		server.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
	}

	// Finally, start the Web server, using a vsock-enabled listener.
	log.Printf("Starting Web server on port %s.", server.Addr)
	var l net.Listener
	if !*ignoreNitro {
		l, err = vsock.Listen(uint32(srvPort))
		if err != nil {
			log.Fatalf("Failed to listen for HTTPS server: %s", err)
		}
		defer func() {
			_ = l.Close()
		}()
	} else {
		l, err = net.Listen("tcp", fmt.Sprintf(":%d", srvPort))
		if err != nil {
			log.Fatalf("Failed to listen for HTTPS server: %s", err)
		}
	}

	if err = server.ServeTLS(l, "", ""); err != nil {
		// ServeTLS always returns a non-nil err.
		fmt.Printf("ServeTLS says: %s", err)
	}
}
