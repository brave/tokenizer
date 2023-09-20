package main

import (
	"errors"
	"time"
)

var errCacheNotReady = errors.New("cache not yet ready")

// cache implements a thread-safe token cache for our Kafka forwarder.
type cache struct {
	in     chan any
	out    chan []any
	length chan int
	done   chan empty
	age    chan time.Time
	conf   *kafkaConfig
}

func newCache() *cache {
	return &cache{
		in:     make(chan any),
		out:    make(chan []any),
		length: make(chan int),
		done:   make(chan empty),
		age:    make(chan time.Time),
	}
}

func (c *cache) start() {
	go func() {
		var (
			age   time.Time
			elems = []any{}
		)
		for {
			select {
			case e := <-c.in:
				if len(elems) == 0 {
					age = time.Now()
				}
				elems = append(elems, e)
			case c.out <- elems:
				elems = []any{}
			case c.length <- len(elems):
			case c.age <- age:
			case <-c.done:
				return
			}
		}
	}()
}

func (c *cache) stop() {
	close(c.done)
}

func (c *cache) len() int {
	return <-c.length
}

func (c *cache) submit(e any) {
	c.in <- e
}

func (c *cache) retrieve() ([]any, error) {
	if c.isReady() {
		return <-c.out, nil
	}
	return nil, errCacheNotReady
}

func (c *cache) isReady() bool {
	age := <-c.age
	if age.IsZero() {
		return false
	}
	// We cache tokens until the cache gets too large or too old -- whichever
	// comes first.
	if c.len() > c.conf.batchSize {
		return true
	}
	if time.Now().Add(-c.conf.batchPeriod).After(age) {
		return true
	}
	return false
}
