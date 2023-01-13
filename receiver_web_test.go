package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	ipv4Addr = "1.2.3.4"
)

func makeReq(t *testing.T, s *httptest.Server, method, path string, h http.Header) *http.Response {
	req, err := http.NewRequest(method, s.URL+path, nil)
	if err != nil {
		t.Fatalf("Failed to create HTTP request: %v", err)
	}
	req.Header = h
	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make HTTP request: %v", err)
	}
	return resp
}

func TestGoodRequest(t *testing.T) {
	walletID := newV4(t)
	expected := clientRequest{
		Addr:   net.ParseIP(ipv4Addr),
		Wallet: walletID,
	}
	inbox := make(chan serializer, 10) // We're using a buffered channel to prevent a deadlock.
	path := fmt.Sprintf("/v2/confirmation/token/%s", walletID)
	srv := httptest.NewServer(newRouter(inbox))
	defer srv.Close()

	resp := makeReq(t, srv, http.MethodGet, path, http.Header{fastlyClientIP: []string{ipv4Addr}})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected HTTP status code %d but got %d.", http.StatusOK, resp.StatusCode)
	}

	r := <-inbox
	received := r.(*clientRequest)
	if !net.IP.Equal(received.Addr, expected.Addr) {
		t.Fatalf("Expected address %q but got %q.", expected.Addr, received.Addr)
	}
	if received.Wallet != expected.Wallet {
		t.Fatalf("Expected wallet %q but got %q.", expected.Wallet, received.Wallet)
	}
}

func TestBadWalletId(t *testing.T) {
	srv := httptest.NewServer(newRouter(make(chan serializer)))
	defer srv.Close()
	badPath := "/v2/confirmation/token/foobar"

	resp := makeReq(t, srv, http.MethodGet, badPath, http.Header{fastlyClientIP: []string{ipv4Addr}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected HTTP status code %d but got %d.", http.StatusBadRequest, resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	received := strings.TrimSpace(string(body))
	expected := errBadWalletFmt.Error()
	if received != expected {
		t.Fatalf("Expected error %q but got %q.", expected, received)
	}
}

func TestNoFastlyHeader(t *testing.T) {
	srv := httptest.NewServer(newRouter(make(chan serializer)))
	defer srv.Close()
	path := fmt.Sprintf("/v2/confirmation/token/%s", newV4(t))

	resp := makeReq(t, srv, http.MethodGet, path, http.Header{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected HTTP status code %d but got %d.", http.StatusBadRequest, resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	received := strings.TrimSpace(string(body))
	expected := errNoFastlyHeader.Error()
	if received != expected {
		t.Fatalf("Expected error %q but got %q.", expected, received)
	}
}

func TestBadFastlyAddr(t *testing.T) {
	srv := httptest.NewServer(newRouter(make(chan serializer)))
	defer srv.Close()
	path := fmt.Sprintf("/v2/confirmation/token/%s", newV4(t))

	resp := makeReq(t, srv, http.MethodGet, path, http.Header{fastlyClientIP: []string{"foo"}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected HTTP status code %d but got %d.", http.StatusBadRequest, resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	received := strings.TrimSpace(string(body))
	expected := errBadFastlyAddrFormat.Error()
	if received != expected {
		t.Fatalf("Expected error %q but got %q.", expected, received)
	}
}
