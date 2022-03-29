package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	msg "github.com/brave-experiments/ia2/message"
	uuid "github.com/satori/go.uuid"
)

func TestFlusher(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	walletID := uuid.NewV4()
	anonAddr := "0123456789"
	keyID := msg.KeyID("foo")

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
				t.Errorf("failed to close request body: %s", err)
			}
		}()

		receivedJSON, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %s", err)
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
