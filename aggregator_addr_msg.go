package main

import (
	"encoding/json"
	"sort"

	uuid "github.com/google/uuid"
)

type ourString string

func (s ourString) bytes() []byte {
	return []byte(s)
}

// empty represents an empty map value.
type empty struct{}

// AddressSet represents a set of string-encoded IP addresses.
type AddressSet map[string]empty

// AddrsByWallet maps a wallet ID to a set of its anonymized IP addresses, all
// represented as strings.
type AddrsByWallet map[uuid.UUID]AddressSet

// WalletsByKeyID maps a key ID to a map of type addrsByWallet.  Key IDs
// represent data collection epochs: whenever the key ID rotates, a new epoch
// begins, and our collection of wallet-to-address records begins afresh.
type WalletsByKeyID map[keyID]AddrsByWallet

// sorted returns the address set's addresses as a sorted string slice.
func (s AddressSet) sorted() []string {
	addrs := []string{}
	for a := range s {
		addrs = append(addrs, a)
	}
	sort.Strings(addrs)
	return addrs
}

// numWallets returns the total number of wallets that are currently in the
// struct.  Note that this may contain duplicate wallets, i.e., wallets that
// are present for key ID x *and* for key ID y.
func (w WalletsByKeyID) numWallets() int {
	if len(w) == 0 {
		return 0
	}

	total := 0
	for _, addrsByWallet := range w {
		total += len(addrsByWallet)
	}
	return total
}

// numAddrs returns the total number of addresses (which may be different from
// the number of unique addresses!) that are currently in the struct.
func (w WalletsByKeyID) numAddrs() int {
	if len(w) == 0 {
		return 0
	}

	total := 0
	for _, addrsByWallet := range w {
		if len(addrsByWallet) == 0 {
			continue
		}
		for _, addrs := range addrsByWallet {
			total += len(addrs)
		}
	}
	return total
}

// MarshalJSON marshals the given key ID-to-wallets map and turns it into the
// following JSON:
//
//	{
//	  "keyid": {
//	    "024752c9-7090-4123-939e-67b08042d7d7": {
//	      "addrs": {
//	        "68a7deb0-615c-4f26-bf87-6b122732d8e9": [
//	          "1.1.1.1",
//	          "2.2.2.2",
//	          ...
//	        ],
//	        ...
//	      }
//	    }
//	  }
//	}
func (w WalletsByKeyID) MarshalJSON() ([]byte, error) {
	type toMarshal struct {
		WalletsByKeyID map[keyID]AddrsByWallet `json:"keyid"`
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
		WalletsByKeyID map[keyID]AddrsByWallet `json:"keyid"`
	}{w})
}

// MarshalJSON marshals the given addresses and turns it into the following
// JSON:
//
//	{
//	  "addrs": {
//	    "68a7deb0-615c-4f26-bf87-6b122732d8e9": [
//	      "1.1.1.1",
//	      "2.2.2.2",
//	      ...
//	    ],
//	    ...
//	  }
//	}
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
			addrSet[addr] = empty{}
		}
		(*a)[walletID] = addrSet
	}
	return nil
}
