package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
	uuid "github.com/satori/go.uuid"
)

const (
	// fastlyClientIP represents the IP address of the client:
	// https://developer.fastly.com/reference/http/http-headers/Fastly-Client-IP/ (retrieved on 2021-11-29)
	fastlyClientIP = "Fastly-Client-IP"
)

var (
	errBadWalletFmt   = errors.New("wallet ID has bad format")
	errNoFastlyHeader = fmt.Errorf("found no %q header", fastlyClientIP)
	errBadAddrFormat  = fmt.Errorf("bad IP address format in %q header", fastlyClientIP)
)

// confTokenHandler takes as input forwarded confirmation token requests from
// Fastly.  We then retrieve the client's wallet ID from the URL, its IP
// address from Fastly's proprietary header, and shove both into our
// anonymization code.
func confTokenHandler(w http.ResponseWriter, r *http.Request) {
	// Make sure that the wallet ID is a valid UUID.
	rawWalletID := chi.URLParam(r, "walletID")
	walletID, err := uuid.FromString(rawWalletID)
	if err != nil {
		http.Error(w, errBadWalletFmt.Error(), http.StatusBadRequest)
		return
	}

	rawAddr := r.Header.Get(fastlyClientIP)
	if rawAddr == "" {
		http.Error(w, errNoFastlyHeader.Error(), http.StatusBadRequest)
		return
	}

	// Fetch the client's IP address from Fastly's proprietary header.
	addr := net.ParseIP(rawAddr)
	if addr == nil {
		http.Error(w, errBadAddrFormat.Error(), http.StatusBadRequest)
		return
	}
	anonymizeAddr(&clientRequest{Addr: addr, Wallet: walletID})
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

	anonymizeAddr(&clientRequest{Addr: addr})
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

// isNonceValid returns true if the given nonce is correctly formatted.
func isNonceValid(nonce string) bool {
	match, _ := regexp.MatchString(nonceRegExp, nonce)
	return match
}

// anonymizeAddr takes as input a client request (consisting of a client's IP
// address and wallet ID) and anonymizes the address via Crypto-PAn or our
// HMAC-based anonymization, depending on what's configured.  Once the address
// is anonymized, the tuple is forwarded to our flushing component.
func anonymizeAddr(req *clientRequest) {
	var anonAddr []byte
	if hmacKey == nil {
		anonAddr = cryptoPAn.Anonymize(req.Addr)
	} else {
		h := hmac.New(sha256.New, hmacKey)
		h.Write([]byte(req.Addr))
		anonAddr = h.Sum(nil)
	}
	req.AnonAddr = anonAddr
	if flusher != nil {
		flusher.Submit(req)
	}
}
