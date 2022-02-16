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

	uuid "github.com/satori/go.uuid"
)

func TestSerialization2(t *testing.T) {
	walletID := uuid.NewV4()
	keyID := KeyID("foo")
	ipAddr := "1.1.1.1"
	batch := walletsByKeyID{
		keyID: addrsByWallet{
			walletID: addressSet{
				ipAddr: empty{},
			},
		},
	}

	serialized, err := json.Marshal(batch)
	if err != nil {
		t.Fatalf("failed to marshal struct: %s", err)
	}

	expected := fmt.Sprintf("{\"keyid\":{\"%s\":{\"addrs\":{\"%s\":[\"%s\"]}}}}",
		keyID, walletID.String(), ipAddr)
	if string(serialized) != expected {
		t.Fatalf("expected %q but got %q", expected, serialized)
	}
}

func TestFlusher(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	walletID := uuid.NewV4()
	ipAddr1 := "1.1.1.1"
	keyID := KeyID("foo")

	expectedPayload := walletsByKeyID{
		keyID: addrsByWallet{
			walletID: addressSet{
				ipAddr1: empty{},
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
			t.Fatalf("received unexpected JSON: %s", receivedJSON)
		}
	}))
	defer srv.Close()

	f := NewFlusher(1, srv.URL)
	defer f.Stop()
	f.Start()
	req := &clientRequest{
		AnonAddr: []byte(ipAddr1),
		Wallet:   walletID,
		KeyID:    keyID,
	}
	f.Submit(req)
	// Submit a duplicate IP address, which should be discarded.
	f.Submit(req)
	wg.Wait()
}
