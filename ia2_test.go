package main

import (
	"crypto/rand"
	"net"
	"os"
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

func TestSetEnvVar(t *testing.T) {
	setEnvVar("foo", "bar")
	if os.Getenv("foo") != "bar" {
		t.Fatal("failed to retrieve previously-set environment variable")
	}
}
