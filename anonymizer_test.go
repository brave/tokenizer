package main

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestAnonymizer(t *testing.T) {
	a := NewAnonymizer(methodCryptoPAn, time.Hour)
	defer a.Stop()

	if a.GetKeyID() != a.GetKeyID() {
		t.Error("Key ID expected to be identical but is different.")
	}

	sum1 := a.GetKeyID()
	// Force a key rotation.
	a.initKeys()
	sum2 := a.GetKeyID()
	if sum1 == sum2 {
		t.Error("Key ID expected to be different after rotation but is identical.")
	}

	addr1, _ := a.Anonymize(net.ParseIP("1.1.1.1"))
	addr2, _ := a.Anonymize(net.ParseIP("1.1.1.1"))
	if !bytes.Equal(addr1, addr2) {
		t.Error("Anonymized addresses expected to be identical.")
	}

	a.initKeys()
	addr3, _ := a.Anonymize(net.ParseIP("1.1.1.1"))
	if bytes.Equal(addr1, addr3) {
		t.Error("Anonymized addresses expected to be different after key rotation.")
	}

	// Now test key rotation by relying on the anonymizer's ticker.
	a = NewAnonymizer(methodHMAC, time.Millisecond)
	defer a.Stop()

	addr1, _ = a.Anonymize(net.ParseIP("1.1.1.1"))
	time.Sleep(time.Millisecond * 5)
	addr2, _ = a.Anonymize(net.ParseIP("1.1.1.1"))
	if bytes.Equal(addr1, addr2) {
		t.Error("Anonymized addresses expected to be different after key rotation.")
	}
}
