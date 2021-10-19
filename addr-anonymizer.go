package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
	"unsafe"

	"github.com/Yawning/cryptopan"
	"github.com/hf/nsm"
	"github.com/hf/nsm/request"
	"github.com/milosgajdos/tenus"
	"github.com/paulbellamy/ratecounter"

	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/sys/unix"
)

const (
	acmeCertCacheDir  = "cert-cache"
	hmacKeySize       = 20
	entropySeedDevice = "/dev/random"
	entropySeedSize   = 2048
)

var certSha256 string
var hmacKey []byte
var cryptoPAn *cryptopan.Cryptopan
var counter = ratecounter.NewRateCounter(1 * time.Second)
var flusher *Flusher

type anonymizerHandler struct {
	handle func(w http.ResponseWriter, r *http.Request)
}

// ServeHTTP increments our rate counter.
func (f anonymizerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	counter.Incr(1)
	f.handle(w, r)
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

// attestationHandler takes as input a nonce and asks the hypervisor to create
// an attestation document that contains the given nonce and our HTTPS
// certificate's SHA-256 hash.  The resulting Base64-encoded attestation
// document is returned to the client.
func attestationHandler(w http.ResponseWriter, r *http.Request) {
	if !isValidRequest(w, r) {
		return
	}
	nonce := r.FormValue("nonce")
	if nonce == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "no nonce given\n")
		return
	}

	rawDoc, err := attest([]byte(nonce), []byte(certSha256), nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "attestation failed: %v\n", err)
		return
	}
	b64Doc := base64.StdEncoding.EncodeToString(rawDoc)
	fmt.Fprintln(w, b64Doc)
}

// submitHandler takes as input an IP address, anonymizes it, and hands it over
// to our flusher, which will send the anonymized IP address to our Kafka
// broker.
func submitHandler(w http.ResponseWriter, r *http.Request) {
	if !isValidRequest(w, r) {
		return
	}
	addrStr := r.FormValue("addr")
	if addrStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "no IP address given\n")
		return
	}

	addr := net.ParseIP(addrStr)
	if addr == nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "invalid IP address format\n")
		return
	}

	var anonAddr []byte
	if hmacKey == nil {
		anonAddr = cryptoPAn.Anonymize(addr)
	} else {
		h := hmac.New(sha256.New, hmacKey)
		h.Write([]byte(addr))
		anonAddr = h.Sum(nil)
	}
	flusher.Submit(anonAddr)
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
		log.Printf("Starting autocert listener.")
		log.Fatal(http.ListenAndServe(":80", certManager.HTTPHandler(nil)))
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
	if !useCryptoPAn {
		log.Println("Using HMAC-SHA256 for IP address anonymization.")
		hmacKey = make([]byte, hmacKeySize)
		_, err := rand.Read(hmacKey)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("HMAC key: %x", hmacKey)
	} else {
		log.Println("Using Crypto-PAn for IP address anonymization.")
		// Determine a cryptographically secure random number that serves as
		// key to our Crypto-PAn object.  The number is determined in the
		// enclave, and therefore unknown to us.
		buf := make([]byte, cryptopan.Size)
		_, err := rand.Read(buf)
		if err != nil {
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

// getNSMRandomness obtains cryptographically secure random bytes from the
// Nitro's NSM and uses them to initialize the system's random number
// generator.  If we don't do that, our system we start with no entropy, which
// means that calls to /dev/(u)random will block.
func getNSMRandomness() error {
	s, err := nsm.OpenDefaultSession()
	if err != nil {
		return err
	}
	defer s.Close()

	fd, err := os.OpenFile(entropySeedDevice, os.O_WRONLY, os.ModePerm)
	if err != nil {
		return err
	}
	defer fd.Close()

	var written int
	for totalWritten := 0; totalWritten < entropySeedSize; {
		// We ignore the error because of a bug that will return an error
		// despite having obtained an attestation document:
		// https://github.com/hf/nsm/issues/2
		res, _ := s.Send(&request.GetRandom{})
		if res.Error != "" {
			return errors.New(string(res.Error))
		}
		if res.GetRandom == nil {
			return errors.New("no GetRandom part in NSM's response")
		}
		if len(res.GetRandom.Random) == 0 {
			return errors.New("got no random bytes from NSM")
		}

		// Write NSM-provided random bytes to the system's entropy pool to seed
		// it.
		if written, err = fd.Write(res.GetRandom.Random); err != nil {
			return err
		}
		totalWritten += written

		// Tell the system to update its entropy count.
		if _, _, errno := unix.Syscall(
			unix.SYS_IOCTL,
			uintptr(fd.Fd()),
			uintptr(unix.RNDADDTOENTCNT),
			uintptr(unsafe.Pointer(&written)),
		); errno != 0 {
			log.Printf("Failed to update system's entropy count: %s", errno)
		}
	}

	log.Println("Initialized the system's entropy pool.")
	return nil
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

func main() {
	var useAcme, debug, useCryptoPAn bool
	var err error
	var fqdn, broker, topic string
	var srvPort, flushInterval int

	flag.BoolVar(&useAcme, "acme", false, "Use ACME to obtain certificates.")
	flag.BoolVar(&debug, "debug", false, "Enable debug mode.")
	flag.BoolVar(&useCryptoPAn, "cryptopan", false, "Use Crypto-PAn anonymization instead of a HMAC.")
	flag.StringVar(&fqdn, "fqdn", "", "FQDN for TLS certificate.")
	flag.StringVar(&broker, "broker", "", "Kafka broker URL to submit anonymized IP addresses to.")
	flag.StringVar(&topic, "topic", "antifraud_verdict_events.production.repsys.upstream", "Kafka topic to submit anonymized IP addresses to.")
	flag.IntVar(&srvPort, "port", 8080, "Port that the server is listening on.")
	flag.IntVar(&flushInterval, "flush", 300, "Time interval after which we flush addresses to the broker.")
	flag.Parse()

	if fqdn == "" {
		log.Fatal("Provide the host's FQDN by using -fqdn.")
	}
	if broker == "" {
		log.Fatal("Provide a Kafka broker URL with -broker.")
	}
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

	if err = getNSMRandomness(); err != nil {
		log.Fatalf("Failed to initialize the system's entropy pool: %s", err)
	}

	if err = assignLoAddr(); err != nil {
		log.Fatalf("Failed to assign address to lo: %s", err)
	}

	log.Println("Setting up HTTP handlers.")
	http.Handle("/attest", anonymizerHandler{attestationHandler})
	http.Handle("/submit", anonymizerHandler{submitHandler})

	initAnonymization(useCryptoPAn)

	log.Printf("Initializing new flusher with interval %ds and broker %s.", flushInterval, broker)
	brokerURL, err := url.Parse(broker)
	if err != nil {
		log.Fatal(err)
	}
	flusher = NewFlusher(flushInterval, *brokerURL, topic)
	flusher.Start()
	defer flusher.Stop()

	server := http.Server{
		Addr: fmt.Sprintf(":%d", srvPort),
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

	// Finally, start the Web server.
	log.Printf("Starting Web server on port %s.", server.Addr)
	if err = server.ListenAndServeTLS("", ""); err != nil {
		log.Fatal(err)
	}
}
