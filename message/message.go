// Package message provides data structures that represent a mapping from
// wallet addresses to anonymized IP addresses.
package message

import (
	"encoding/json"
	"fmt"

	uuid "github.com/satori/go.uuid"
)

// Empty represents an empty map value.
type Empty struct{}

// AddressSet represents a set of string-encoded IP addresses.
type AddressSet map[string]Empty

// AddrsByWallet maps a wallet ID to a set of its anonymized IP addresses, all
// represented as strings.
type AddrsByWallet map[uuid.UUID]AddressSet

// KeyID is a UUID that's derived from the anonymizer's current key.
type KeyID struct {
	uuid.UUID
}

// WalletsByKeyID maps a key ID to a map of type addrsByWallet.  Key IDs
// represent data collection epochs: whenever the key ID rotates, a new epoch
// begins, and our collection of wallet-to-address records begins afresh.
type WalletsByKeyID map[KeyID]AddrsByWallet

// MarshalJSON marshals the given key ID-to-wallets map and turns it into the
// following JSON:
//
// {
//   "keyid": {
//     "024752c9-7090-4123-939e-67b08042d7d7": {
//       "addrs": {
//         "68a7deb0-615c-4f26-bf87-6b122732d8e9": [
//           "1.1.1.1",
//           "2.2.2.2",
//           ...
//         ],
//         ...
//       }
//     }
//   }
// }
func (w WalletsByKeyID) MarshalJSON() ([]byte, error) {
	type toMarshal struct {
		WalletsByKeyID map[KeyID]AddrsByWallet `json:"keyid"`
	}
	m := &toMarshal{WalletsByKeyID: make(WalletsByKeyID)}
	for keyID, wallets := range w {
		m.WalletsByKeyID[keyID] = wallets
	}
	return json.Marshal(m)
}

// UnmarshalJSON unmarshalls the given JSON and turns it back into the given
// WalletsByKeyID.
func (w WalletsByKeyID) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &struct {
		WalletsByKeyID map[KeyID]AddrsByWallet `json:"keyid"`
	}{w})
}

// MarshalJSON marshals the given addresses and turns it into the following
// JSON:
//
// {
//   "addrs": {
//     "68a7deb0-615c-4f26-bf87-6b122732d8e9": [
//       "1.1.1.1",
//       "2.2.2.2",
//       ...
//     ],
//     ...
//   }
// }
func (a AddrsByWallet) MarshalJSON() ([]byte, error) {
	type toMarshal struct {
		Addrs map[string][]string `json:"addrs"`
	}
	m := &toMarshal{Addrs: make(map[string][]string)}
	for wallet, addrSet := range a {
		addrSlice := []string{}
		for addr := range addrSet {
			addrSlice = append(addrSlice, addr)
		}
		m.Addrs[wallet.String()] = addrSlice
	}
	return json.Marshal(m)
}

// UnmarshalJSON unmarshalls the given JSON and turns it back into the given
// AddrsByWallet.
func (a *AddrsByWallet) UnmarshalJSON(data []byte) error {
	*a = make(AddrsByWallet)
	// Create an anonymous struct into which we're going to unmarshal the given
	// data bytes.
	s := &struct {
		AddrsByWallet map[uuid.UUID][]string `json:"addrs"`
	}{
		AddrsByWallet: make(map[uuid.UUID][]string),
	}
	if err := json.Unmarshal(data, s); err != nil {
		return err
	}

	// Now add the key/value pairs from our anonymous struct to our
	// AddrsByWallet.
	for walletID, addrs := range s.AddrsByWallet {
		addrSet := make(AddressSet)
		for _, addr := range addrs {
			addrSet[addr] = Empty{}
		}
		(*a)[walletID] = addrSet
	}
	return nil
}

// String implements the Stringer interface for walletsByKeyID.
func (w WalletsByKeyID) String() string {
	var s string
	for keyID, wallets := range w {
		s += fmt.Sprintf("Key ID %s: %s\n", keyID, wallets)
	}
	return s
}

// String implements the Stringer interface for addrsByWallet.
func (a AddrsByWallet) String() string {
	allAddrSet := make(map[string]Empty)
	for _, addrSet := range a {
		for a := range addrSet {
			allAddrSet[a] = Empty{}
		}

	}
	return fmt.Sprintf("Holding %d wallet addresses containing a total of %d unique IP addresses.", len(a), len(allAddrSet))
}
