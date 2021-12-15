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
