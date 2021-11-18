package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"
	"unsafe"

	"github.com/Yawning/cryptopan"
	"github.com/hf/nsm"
	"github.com/hf/nsm/request"
	"github.com/mdlayher/vsock"
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

// isNonceValid returns true if the given nonce is correctly formatted.
func isNonceValid(nonce string) bool {
	match, _ := regexp.MatchString(nonceRegExp, nonce)
	return match
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
	if !isNonceValid(nonce) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "bad nonce format\n")
		return
	}
	// Decode hex-encoded nonce.
	rawNonce, err := hex.DecodeString(nonce)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "failed to decode nonce\n")
		return
	}

	rawDoc, err := attest(rawNonce, []byte(certSha256), nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "attestation failed: %v\n", err)
		return
	}
	b64Doc := base64.StdEncoding.EncodeToString(rawDoc)
	fmt.Fprintln(w, b64Doc)
}

// forwardHandler takes as input forwarded requests from Fastly.  Those
// requests contain an HTTP header x-forwarded-for that carries the client's IP
// address.
func forwardHandler(w http.ResponseWriter, r *http.Request) {
	if !isValidRequest(w, r) {
		return
	}

	addr := net.ParseIP(r.Header.Get("x-forwarded-for"))
	if addr == nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "invalid IP address format\n")
		return
	}
	anonymizeAddr(addr)
}

// anonymizeAddr takes as input an IP address and anonymizes the address via
// Crypto-PAn or our HMAC-based anonymization, depending on what's configured.
// Once the address is anonymized, it's forwarded to our flushing component.
func anonymizeAddr(addr net.IP) {
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

	anonymizeAddr(addr)
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

// getNSMRandomness obtains cryptographically secure random bytes from the
// Nitro's NSM and uses them to initialize the system's random number
// generator.  If we don't do that, our system we start with no entropy, which
// means that calls to /dev/(u)random will block.
func getNSMRandomness() error {
	s, err := nsm.OpenDefaultSession()
	if err != nil {
		return err
	}
	defer func() {
		_ = s.Close()
	}()

	fd, err := os.OpenFile(entropySeedDevice, os.O_WRONLY, os.ModePerm)
	if err != nil {
		return err
	}
	defer func() {
		if err = fd.Close(); err != nil {
			log.Printf("Failed to close %q: %s", entropySeedDevice, err)
		}
	}()

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
	http.Handle("/forward", anonymizerHandler{forwardHandler})

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

	// Finally, start the Web server, using a vsock-enabled listener.
	log.Printf("Starting Web server on port %s.", server.Addr)
	l, err := vsock.Listen(uint32(srvPort))
	if err != nil {
		log.Fatalf("Failed to listen for HTTPS server: %s", err)
	}
	defer func() {
		_ = l.Close()
	}()

	if err = server.ServeTLS(l, "", ""); err != nil {
		// ServeTLS always returns a non-nil err.
		fmt.Printf("ServeTLS says: %s", err)
	}
}
