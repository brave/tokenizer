package main

import (
	"bytes"
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	uuid "github.com/google/uuid"
)

func TestCompileKafkaMsg(t *testing.T) {
	keyID := keyID{UUID: uuid.New()}
	walletID := uuid.New()
	addr1, addr2 := "1.1.1.1", "2.2.2.2"
	addrs := AddressSet{
		addr1: empty{},
		addr2: empty{},
	}

	msg, err := compileKafkaMsg(keyID, walletID, addrs)
	if err != nil {
		t.Fatalf("Expected no error but got: %v", err)
	}

	justification := `{\"keyid\":\"` + keyID.String() +
		`\",\"addrs\":[\"` + addr1 + `\",\"` + addr2 + `\"]}`
	expectedJSON := fmt.Sprintf("{\"wallet_id\":\"%s\","+
		"\"service\":\"%s\","+
		"\"signal\":\"%s\","+
		"\"score\":0,"+
		"\"justification\":\"%s\","+
		"\"created_at\":\"%s\"}",
		walletID,
		schemaService,
		schemaSignal,
		justification,
		time.Now().UTC().Format(time.RFC3339),
	)

	expectedMsg, err := avroEncode(ourCodec, []byte(expectedJSON))
	if err != nil {
		t.Fatalf("Failed to encode our JSON to Avro: %v", err)
	}

	if !bytes.Equal(msg, []byte(expectedMsg)) {
		t.Fatalf("Expected\n%s\nbut got\n%s", expectedMsg, msg)
	}
}

func TestAddrAggregatorProcess(t *testing.T) {
	rawAddr1 := "1.2.3.4"
	rawAddr2 := "2.3.4.5"
	addr1 := net.ParseIP(rawAddr1)
	addr2 := net.ParseIP(rawAddr2)
	wallet1 := newV4(t)
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
