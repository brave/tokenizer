package main

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	// Label keys and values.
	httpCode = "code"
	httpBody = "body"
	outcome  = "outcome"
	success  = "success"
	fail     = "fail"

	// Our Prometheus namespace.
	ns = "tokenizer"
)

// metrics contains Prometheus metrics for the components we use in production,
// i.e., the Web receiver, the IP address aggregator, the Crypto-PAn tokenizer,
// and the Kafka forwarder.
type metrics struct {
	// The number of addresses and wallets that our address aggregator is
	// currently waiting to flush.
	numWallets   prometheus.Gauge
	numAddrs     prometheus.Gauge
	webResponses *prometheus.CounterVec
	numForwarded *prometheus.CounterVec
	numTokenized *prometheus.CounterVec
}

// failBecause turns the given error into a string that's ready to be used as a
// Prometheus label value, e.g., "foo crashed" is turned into "fail (foo
// crashed)".
func failBecause(err error) string {
	return fmt.Sprintf("fail (%s)", err.Error())
}

// init initializes our Prometheus metrics.
func init() {
	m.numWallets = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "num_wallets",
		Help:      "The number of wallets that the address aggregator currently stores",
	})
	m.numAddrs = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "num_addrs",
		Help:      "The number of addresses that the address aggregator currently stores",
	})

	m.webResponses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "web_responses",
			Help:      "HTTP responses of the Web receiver",
		},
		[]string{httpCode, httpBody},
	)
	m.numForwarded = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "num_forwarded",
			Help:      "(Un)successfully forwarded tokens using Kafka",
		},
		[]string{outcome},
	)
	m.numTokenized = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "num_tokenized",
			Help:      "Crypto-PAn's (un)successfully tokenize'd blobs",
		},
		[]string{outcome},
	)
}
