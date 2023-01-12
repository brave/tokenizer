package main

import (
	"bufio"
	"os"
)

// stdinReceiver implements a receiver that reads data from stdin.
type stdinReceiver struct {
	i    chan serializer
	done chan empty
}

func newStdinReceiver() receiver {
	return &stdinReceiver{
		i:    make(chan serializer),
		done: make(chan empty),
	}
}

func (s *stdinReceiver) setConfig(c *config) {}

func (s *stdinReceiver) inbox() chan serializer {
	return s.i
}

func (s *stdinReceiver) start() {
	stdin := make(chan string)
	go func(c chan string) {
		reader := bufio.NewReader(os.Stdin)
		for {
			str, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			stdin <- str
		}
	}(stdin)

	go func() {
		for {
			select {
			case <-s.done:
				return
			case str := <-stdin:
				s.i <- ourString(str)
				l.Printf("Sent received data to aggregator.")
			}
		}
	}()
}

func (s *stdinReceiver) stop() {
	close(s.done)
}
