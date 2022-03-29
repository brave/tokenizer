package message

import (
	"encoding/json"
	"fmt"
	"testing"

	uuid "github.com/satori/go.uuid"
)

func TestSerialization2(t *testing.T) {
	walletID := uuid.NewV4()
	keyID := KeyID("foo")
	ipAddr := "1.1.1.1"
	batch := WalletsByKeyID{
		keyID: AddrsByWallet{
			walletID: AddressSet{
				ipAddr: Empty{},
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

	// Now turn the raw JSON data back into a struct.
	var newBatch = make(WalletsByKeyID)
	if err := json.Unmarshal(serialized, &newBatch); err != nil {
		t.Fatalf("failed to unmarshal JSON: %s", err)
	}

	if len(newBatch) != len(batch) {
		t.Fatalf("old and new batch don't have same number of key IDs (%d and %d)",
			len(newBatch), len(batch))
	}
	if len(newBatch[keyID]) != len(batch[keyID]) {
		t.Fatalf("old and new batch don't have same number of wallets (%d and %d)",
			len(newBatch[keyID]), len(batch[keyID]))
	}
	if len(newBatch[keyID][walletID]) != len(batch[keyID][walletID]) {
		t.Fatalf("old and new batch don't have same number of addresses (%d and %d)",
			len(newBatch[keyID][walletID]), len(batch[keyID][walletID]))
	}
	if newBatch[keyID][walletID][ipAddr] != batch[keyID][walletID][ipAddr] {
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

	// Finally, test our string representations.
	if batch.String() != newBatch.String() {
		t.Fatalf("expected string representation %q but got %q", batch, newBatch)
	}
}
