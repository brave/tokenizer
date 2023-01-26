package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	uuid "github.com/google/uuid"
)

const (
	// fastlyClientIP represents the IP address of the client:
	// https://developer.fastly.com/reference/http/http-headers/Fastly-Client-IP/
	// (retrieved on 2021-11-29)
	fastlyClientIP = "Fastly-Client-IP"
)

var (
	errBadWalletFmt        = errors.New("wallet ID has bad format")
	errNoFastlyHeader      = fmt.Errorf("found no %q header", fastlyClientIP)
	errBadFastlyAddrFormat = fmt.Errorf("bad IP address format in %q header", fastlyClientIP)
)

// clientRequest represents a client's confirmation token request.  It contains
// the client's IP address and wallet ID.
type clientRequest struct {
	Addr   net.IP    `json:"addr"`
	Wallet uuid.UUID `json:"wallet"`
}

func (c *clientRequest) bytes() []byte {
	return c.Addr
}

// webReceiver implements a receiver that exposes an HTTP API to receive data.
type webReceiver struct {
	done   chan empty
	in     chan serializer
	router *chi.Mux
	port   uint16
}

func newWebReceiver() receiver {
	w := &webReceiver{
		in:   make(chan serializer),
		done: make(chan empty),
	}
	w.router = newRouter(w.in)

	return w
}

func newRouter(inbox chan serializer) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/v1/confirmation/token/{walletID}", getConfTokenHandler(inbox))
	r.Get("/v2/confirmation/token/{walletID}", getConfTokenHandler(inbox))
	return r
}

func (w *webReceiver) setConfig(c *config) {
	w.port = c.port
}

func (w *webReceiver) inbox() chan serializer {
	return w.in
}

func (w *webReceiver) start() {
	go func() {
		srv := &http.Server{
			Addr:    fmt.Sprintf(":%d", w.port),
			Handler: w.router,
		}
		l.Fatal(srv.ListenAndServe())
	}()
}

func (w *webReceiver) stop() {
	close(w.done)
}

func getConfTokenHandler(inbox chan serializer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Make sure that the wallet ID is a valid UUID.
		rawWalletID := chi.URLParam(r, "walletID")

		walletID, err := uuid.Parse(rawWalletID)
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

		inbox <- &clientRequest{Addr: addr, Wallet: walletID}
		l.Printf("Sent received data to aggregator.")
	}
}