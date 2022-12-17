package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	msg "github.com/brave/ia2/message"
	uuid "github.com/satori/go.uuid"
)

func TestFlusher(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	walletID := uuid.NewV4()
	anonAddr := "0123456789"
	keyID := msg.KeyID{UUID: uuid.NewV4()}

	expectedPayload := msg.WalletsByKeyID{
		keyID: msg.AddrsByWallet{
			walletID: msg.AddressSet{
				fmt.Sprintf("%x", anonAddr): msg.Empty{},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer wg.Done()
		defer func() {
			if err := r.Body.Close(); err != nil {
				t.Errorf("failed to close request body: %v", err)
			}
		}()

		receivedJSON, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}

		expectedJSON, err := json.Marshal(expectedPayload)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(receivedJSON, expectedJSON) {
			t.Fatalf("received unexpected JSON:\n%s\n%s", receivedJSON, expectedJSON)
		}
	}))
	defer srv.Close()

	f := NewFlusher(1, srv.URL)
	defer f.Stop()
	f.Start()
	req := &clientRequest{
		AnonAddr: []byte(anonAddr),
		Wallet:   walletID,
		KeyID:    keyID,
	}
	f.Submit(req)
	// Submit a duplicate IP address, which should be discarded.
	f.Submit(req)
	wg.Wait()
}
