package main

import (
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	c := newCache()
	c.conf = &kafkaConfig{
		batchPeriod: time.Second,
		batchSize:   2,
	}
	go c.start()
	defer c.stop()

	assertEqual(t, c.isReady(), false)
	_, err := c.retrieve()
	assertEqual(t, err, errCacheNotReady)
	assertEqual(t, c.len(), 0)

	c.submit(token([]byte("foo")))
	assertEqual(t, c.isReady(), false)

	c.submit(token([]byte("bar")))
	assertEqual(t, c.isReady(), false)

	c.submit(token([]byte("baz")))
	assertEqual(t, c.isReady(), true)
	assertEqual(t, c.len(), 3)

	elems, err := c.retrieve()
	assertEqual(t, err, nil)
	assertEqual(t, len(elems), 3)

	assertEqual(t, c.isReady(), false)
	assertEqual(t, c.len(), 0)
}

func TestIsReady(t *testing.T) {
	c := newCache()
	c.conf = &kafkaConfig{
		batchPeriod: time.Second,
		batchSize:   2,
	}
	c.start()
	defer c.stop()

	c.submit(token([]byte("foo")))
	assertEqual(t, c.isReady(), false)

	// Exceed cache size.
	c.submit(token([]byte("bar")))
	c.submit(token([]byte("baz")))
	assertEqual(t, c.isReady(), true)

	_, _ = c.retrieve()
	assertEqual(t, c.isReady(), false)
}
