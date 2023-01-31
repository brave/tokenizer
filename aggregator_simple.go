package main

// simpleAggregator implements an aggregator that does nothing but tokenizing
// incoming data.
type simpleAggregator struct {
	t      tokenizer
	inbox  chan serializer
	outbox chan token
	done   chan empty
}

func newSimpleAggregator() aggregator {
	return &simpleAggregator{
		done: make(chan empty),
	}
}

func (s *simpleAggregator) setConfig(c *config) {}

func (s *simpleAggregator) use(t tokenizer) {
	s.t = t
}

func (s *simpleAggregator) connect(inbox chan serializer, outbox chan token) {
	s.inbox = inbox
	s.outbox = outbox
}

func (s *simpleAggregator) start() {
	if err := s.t.resetKey(); err != nil {
		l.Fatalf("Failed to reset tokenizer key: %v", err)
	}

	go func() {
		for {
			select {
			case <-s.done:
				return
			case b := <-s.inbox:
				token, err := s.t.tokenize(b)
				if err != nil {
					l.Printf("Failed to tokenize blob: %v", err)
					continue
				}
				l.Println("Tokenized blob.")
				s.outbox <- token
				l.Println("Sent token to forwarder.")
			}
		}
	}()
}

func (s *simpleAggregator) stop() {
	close(s.done)
	l.Println("Stopped aggregator.")
}
