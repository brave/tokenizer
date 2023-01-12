package main

import "fmt"

// stdoutForwarder implements a forwarder that prints all data to stdout.
type stdoutForwarder struct {
	out  chan token
	done chan empty
}

func newStdoutForwarder() forwarder {
	return &stdoutForwarder{
		out:  make(chan token),
		done: make(chan empty),
	}
}

func (s *stdoutForwarder) setConfig(c *config) {}

func (s *stdoutForwarder) outbox() chan token {
	return s.out
}

func (s *stdoutForwarder) start() {
	go func() {
		for {
			select {
			case <-s.done:
				return
			case t := <-s.out:
				l.Println("Received token from aggregator.")
				fmt.Println(string(t))
			}
		}
	}()
}

func (s *stdoutForwarder) stop() {
	close(s.done)
}
