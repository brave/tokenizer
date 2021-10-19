package main

import (
	"log"
	"net"

	"github.com/mdlayher/vsock"
)

const (
	parentCID = 3
)

type vProxy struct {
	raddr *vsock.Addr
	laddr *net.TCPAddr
}

func (p *vProxy) Start(done chan bool) error {
	// Bind to TCP address.
	ln, err := net.Listen("tcp", "127.0.0.1:1080")
	if err != nil {
		return err
	}
	done <- true // Signal to caller that we're ready to accept connections.

	for {

		log.Printf("Waiting for new outgoing TCP connection.")
		lconn, err := ln.Accept()
		if err != nil {
			log.Printf("Failed to accept proxy connection: %s", err)
			continue
		}
		log.Printf("Accepted new outgoing TCP connection.")

		// Establish connection with SOCKS proxy via our vsock interface.
		rconn, err := vsock.Dial(p.raddr.ContextID, p.raddr.Port)
		if err != nil {
			log.Printf("Failed to establish connection to SOCKS proxy: %s", err)
			continue
		}
		log.Println("Established connection with SOCKS proxy over vsock.")

		// Now pipe data from left to right and vice versa.
		go p.pipe(lconn, rconn)
		go p.pipe(rconn, lconn)
	}
}

func (p *vProxy) pipe(src, dst net.Conn) {
	defer func() {
		if err := src.Close(); err != nil {
			log.Printf("Failed to close connection: %s", err)
		}
	}()
	buf := make([]byte, 0xffff)
	for {
		n, err := src.Read(buf)
		if err != nil {
			log.Printf("Failed to read from src connection: %s", err)
			return
		}
		b := buf[:n]
		n, err = dst.Write(b)
		if err != nil {
			log.Printf("Failed to write to dst connection: %s", err)
			return
		}
		if n != len(b) {
			log.Printf("Only wrote %d out of %d bytes.", n, len(b))
			return
		}
	}
}

// NewVProxy returns a new vProxy instance.
func NewVProxy() (*vProxy, error) {
	laddr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:1080")
	if err != nil {
		return nil, err
	}

	return &vProxy{
		raddr: &vsock.Addr{ContextID: parentCID, Port: 1080},
		laddr: laddr,
	}, nil
}
