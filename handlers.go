package main

// This file implements HTTP handlers that take as input IP addresses that are
// then submitted to the flusher (see flusher.go).

import (
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	uuid "github.com/satori/go.uuid"
)

const (
	// fastlyClientIP represents the IP address of the client:
	// https://developer.fastly.com/reference/http/http-headers/Fastly-Client-IP/ (retrieved on 2021-11-29)
	fastlyClientIP = "Fastly-Client-IP"
)

var (
	errBadWalletFmt        = errors.New("wallet ID has bad format")
	errNoFastlyHeader      = fmt.Errorf("found no %q header", fastlyClientIP)
	errBadFastlyAddrFormat = fmt.Errorf("bad IP address format in %q header", fastlyClientIP)
	errNoAddr              = errors.New("could not find addr in POST form data")
	errBadAddrFormat       = errors.New("failed to parse given IP address")
)

// clientRequest represents a client's confirmation token request.  It contains
// the client's IP address, wallet ID, and we augment it with the client's
// anonymized IP address and the corresponding key ID.
type clientRequest struct {
	Addr     net.IP
	AnonAddr []byte
	KeyID    KeyID
	Wallet   uuid.UUID
}

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
		http.Error(w, errBadFastlyAddrFormat.Error(), http.StatusBadRequest)
		return
	}
	anonymizeAddr(&clientRequest{Addr: addr, Wallet: walletID})
}

// addressHandler takes as input an IP address, anonymizes it, and hands it over
// to our flusher, which will send the anonymized IP address to our Kafka
// broker.
func addressHandler(w http.ResponseWriter, r *http.Request) {
	addrStr := r.PostFormValue("addr")
	if addrStr == "" {
		http.Error(w, errNoAddr.Error(), http.StatusBadRequest)
		return
	}
	addr := net.ParseIP(addrStr)
	if addr == nil {
		http.Error(w, errBadAddrFormat.Error(), http.StatusBadRequest)
		return
	}

	anonymizeAddr(&clientRequest{Addr: addr})
}

// anonymizeAddr takes as input a client request (consisting of a client's IP
// address and wallet ID) and anonymizes the address via Crypto-PAn or our
// HMAC-based anonymization, depending on what's configured.  Once the address
// is anonymized, the tuple is forwarded to our flushing component.
func anonymizeAddr(req *clientRequest) {
	req.AnonAddr, req.KeyID = anonymizer.Anonymize(req.Addr)
	if flusher != nil {
		flusher.Submit(req)
	}
}
