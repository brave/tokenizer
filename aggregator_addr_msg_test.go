package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestOurString(t *testing.T) {
	s := "foo"
	o := ourString(s)
	if !bytes.Equal(o.bytes(), []byte(s)) {
		t.Fatalf("Expected %s but got %s.", []byte(s), o.bytes())
	}
}

func TestSorted(t *testing.T) {
	a1 := "1.1.1.1"
	a2 := "2.2.2.2"
	a3 := "3.3.3.3"
	a4 := "4.4.4.4"
	a5 := "5.5.5.5"

	s := AddressSet{
		a5: empty{},
		a2: empty{},
		a4: empty{},
		a1: empty{},
		a3: empty{},
	}

	assertEqual(t,
		strings.Join(s.sorted(), ""),
		strings.Join([]string{a1, a2, a3, a4, a5}, ""),
	)
}

func TestSerialization(t *testing.T) {
	walletID := newV4(t)
	kID := keyID{UUID: newV4(t)}
	ipAddr := "1.1.1.1"
	batch := WalletsByKeyID{
		kID: AddrsByWallet{
			walletID: AddressSet{
				ipAddr: empty{},
			},
		},
	}

	serialized, err := json.Marshal(batch)
	if err != nil {
		t.Fatalf("failed to marshal struct: %s", err)
	}

	expected := fmt.Sprintf("{\"keyid\":{\"%s\":{\"addrs\":{\"%s\":[\"%s\"]}}}}",
		kID, walletID.String(), ipAddr)
	if string(serialized) != expected {
		t.Fatalf("expected %q but got %q", expected, serialized)
	}

	// Now turn the raw JSON data back into a struct.
	var newBatch = make(WalletsByKeyID)
	if err := json.Unmarshal(serialized, &newBatch); err != nil {
		t.Fatalf("failed to unmarshal JSON: %s", err)
	}

	if len(newBatch) != len(batch) {
		t.Fatalf("old and new batch don't have same number of key IDs (%d and %d)",
			len(newBatch), len(batch))
	}
	if len(newBatch[kID]) != len(batch[kID]) {
		t.Fatalf("old and new batch don't have same number of wallets (%d and %d)",
			len(newBatch[kID]), len(batch[kID]))
	}
	if len(newBatch[kID][walletID]) != len(batch[kID][walletID]) {
		t.Fatalf("old and new batch don't have same number of addresses (%d and %d)",
			len(newBatch[kID][walletID]), len(batch[kID][walletID]))
	}
	if newBatch[kID][walletID][ipAddr] != batch[kID][walletID][ipAddr] {
		t.Fatal("unmarshalled JSON not as expected")
	}

	// Marshal newly unmarshalled JSON and make sure that it's as expected.
	newSerialized, err := json.Marshal(newBatch)
	if err != nil {
		t.Fatalf("failed to marshal struct: %s", err)
	}
	if string(newSerialized) != expected {
		t.Fatalf("expected %q but got %q", expected, newSerialized)
	}
}

func TestNumWalletsAndAddrs(t *testing.T) {
	w := WalletsByKeyID{
		keyID{newV4(t)}: AddrsByWallet{
			newV4(t): AddressSet{
				"1.1.1.1": empty{},
			},
		},
		keyID{newV4(t)}: AddrsByWallet{
			newV4(t): AddressSet{
				"1.1.1.1": empty{},
			},
			newV4(t): AddressSet{
				"2.2.2.2": empty{},
				"3.3.3.3": empty{},
			},
			newV4(t): AddressSet{
				"3.3.3.3": empty{},
				"4.4.4.4": empty{},
				"5.5.5.5": empty{},
			},
			newV4(t): AddressSet{},
		},
	}

	assertEqual(t, w.numWallets(), 5)
	assertEqual(t, w.numAddrs(), 7)
}
