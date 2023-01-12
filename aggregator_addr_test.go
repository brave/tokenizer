package main

import (
	"net"
	"reflect"
	"testing"

	uuid "github.com/satori/go.uuid"
)

func TestAddrAggregatorProcess(t *testing.T) {
	rawAddr1 := "1.2.3.4"
	rawAddr2 := "2.3.4.5"
	addr1 := net.ParseIP(rawAddr1)
	addr2 := net.ParseIP(rawAddr2)
	wallet1 := uuid.NewV4()
	tokenizer := newVerbatimTokenizer()
	_ = tokenizer.resetKey()
	a := newAddrAggregator()
	a.use(tokenizer)
	kID := tokenizer.keyID()

	tests := []struct {
		reqs  []*clientRequest
		addrs WalletsByKeyID
	}{
		// One wallet mapping to one address.
		{
			[]*clientRequest{
				{
					Addr:   addr1,
					Wallet: wallet1,
				},
			},
			WalletsByKeyID{
				*kID: AddrsByWallet{
					wallet1: AddressSet{
						ipv4Addr: empty{},
					},
				},
			},
		},
		// One wallet mapping to two addresses.
		{
			[]*clientRequest{
				{
					Addr:   addr1,
					Wallet: wallet1,
				},
				{
					Addr:   addr2,
					Wallet: wallet1,
				},
			},
			WalletsByKeyID{
				*kID: AddrsByWallet{
					wallet1: AddressSet{
						rawAddr1: empty{},
						rawAddr2: empty{},
					},
				},
			},
		},
	}

	for _, test := range tests {
		addrAggr := a.(*addrAggregator)
		for _, req := range test.reqs {
			_ = addrAggr.processRequest(req)
		}
		if !reflect.DeepEqual(addrAggr.addrs, test.addrs) {
			t.Fatalf("Expected %+v but got %+v.", test.addrs, addrAggr.addrs)
		}
	}
}
