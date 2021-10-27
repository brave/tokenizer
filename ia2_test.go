package main

import (
	"crypto/rand"
	"net"
	"testing"

	"github.com/Yawning/cryptopan"
)

func BenchmarkAnonymizingPerformance(b *testing.B) {
	var addr net.IP
	var buf = make([]byte, cryptopan.Size)
	_, err := rand.Read(buf)
	if err != nil {
		b.Fatal(err)
	}
	ctx, err := cryptopan.New(buf)
	if err != nil {
		b.Fatal(err)
	}
	addr = net.ParseIP("1.1.1.1")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ctx.Anonymize(addr)
	}
}

func TestIsNonceValid(t *testing.T) {
	if isNonceValid("") {
		t.Error("empty nonce was mistakenly accepted")
	}
	if isNonceValid("XRsp9vT9CDEZF3tu4xDRZ1Dnmayyc1bwKQ3f+Q==") {
		t.Error("base64 nonce was mistakenly accepted")
	}
	if !isNonceValid("0000000000000000000000000000000000000000") {
		t.Error("all-0 nonce was mistakenly rejected")
	}
	if !isNonceValid("0123456789abcdef0123456789abcdef01234567") {
		t.Error("hex nonce was mistakenly rejected")
	}
}
