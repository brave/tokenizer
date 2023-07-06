package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"time"

	uuid "github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	tokenizerCryptoPAn = "cryptopan"
	tokenizerHmac      = "hmac"
	tokenizerVerbatim  = "verbatim"

	forwarderStdout = "stdout"
	forwarderKafka  = "kafka"

	receiverWeb   = "web"
	receiverStdin = "stdin"

	aggregatorSimple = "simple"
	aggregatorAddr   = "address"

	defaultTokenizer  = tokenizerHmac
	defaultForwarder  = forwarderStdout
	defaultReceiver   = receiverStdin
	defaultAggregator = aggregatorSimple
)

var (
	l = log.New(os.Stderr, "tknzr: ", log.Ldate|log.Ltime|log.LUTC|log.Lshortfile)
	// Pre-defined UUID namespaces aren't a great fit for our use case, so we
	// use our own namespace, based on a randomly-generated V4 UUID.
	uuidNamespace = uuid.MustParse("c298cccd-3c75-4e72-a73b-47811ac13f4f")
	ourReceivers  = map[string]func() receiver{
		receiverStdin: newStdinReceiver,
		receiverWeb:   newWebReceiver,
	}
	ourAggregators = map[string]func() aggregator{
		aggregatorSimple: newSimpleAggregator,
		aggregatorAddr:   newAddrAggregator,
	}
	ourForwarders = map[string]func() forwarder{
		forwarderStdout: newStdoutForwarder,
		forwarderKafka:  newKafkaForwarder,
	}
	ourTokenizers = map[string]func() tokenizer{
		tokenizerHmac:      newHmacTokenizer,
		tokenizerCryptoPAn: newCryptoPAnTokenizer,
		tokenizerVerbatim:  newVerbatimTokenizer,
	}
	m = metrics{}
)

func bootstrap(c *config, comp *components, done chan empty) {
	// Propagate our configuration to all components.
	comp.a.setConfig(c)
	comp.r.setConfig(c)
	comp.f.setConfig(c)

	// Tell the aggregator what tokenizer to use.
	comp.a.use(comp.t)
	// Tell the aggregator where to get data and where to send it to.
	comp.a.connect(comp.r.inbox(), comp.f.outbox())

	// Start all components.
	comp.a.start()
	defer comp.a.stop()
	comp.r.start()
	defer comp.r.stop()
	comp.f.start()
	defer comp.f.stop()

	l.Println("Done bootstrapping.  Now waiting for channel to close.")
	<-done
}

func parseFlags(progname string, args []string) (*components, *config, error) {
	var err error
	var exposePrometheus bool
	var tokenizer, forwarder, aggregator, receiver string
	var rawFwdInterval, rawKeyExpiry, port, prometheusPort int

	fs := flag.NewFlagSet(progname, flag.ContinueOnError)

	fs.BoolVar(&exposePrometheus, "expose-prometheus", false,
		"Expose Prometheus metrics.")
	fs.IntVar(&prometheusPort, "prometheus-port", 9090,
		"Make Prometheus metrics available at http://0.0.0.0:<port>/metrics.")
	fs.IntVar(&rawFwdInterval, "forward-interval", 60*5,
		"Number of seconds after which data is forwarded to backend.")
	fs.IntVar(&rawKeyExpiry, "key-expiry", 60*60*24*30*6,
		"Number of seconds after which keys are rotated.")
	fs.IntVar(&port, "port", 8080,
		"Port the Web receiver should listen on.")
	fs.StringVar(&tokenizer, "tokenizer", defaultTokenizer,
		"The name of the tokenizer to use.")
	fs.StringVar(&forwarder, "forwarder", defaultForwarder,
		"The name of the forwarder to use.")
	fs.StringVar(&aggregator, "aggregator", defaultAggregator,
		"The name of the aggregator to use.")
	fs.StringVar(&receiver, "receiver", defaultReceiver,
		"The name of the receiver to use.")
	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}

	c := &config{}
	// Parse configuration flags.
	if port < 1 || port > math.MaxUint16 {
		return nil, nil, fmt.Errorf("port must be in interval [1, %d]", math.MaxUint16)
	}
	c.port = uint16(port)
	c.keyExpiry, err = time.ParseDuration(fmt.Sprintf("%ds", rawKeyExpiry))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse key expiration: %w", err)
	}
	c.fwdInterval, err = time.ParseDuration(fmt.Sprintf("%ds", rawFwdInterval))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse forward interval: %w", err)
	}
	if forwarder == forwarderKafka {
		c.kafkaConfig, err = loadKafkaConfig()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse Kafka config: %w", err)
		}
	}
	if prometheusPort < 1 || prometheusPort > math.MaxUint16 {
		return nil, nil, fmt.Errorf("Prometheus port must be in interval [1, %d]", math.MaxUint16)
	}
	if exposePrometheus && receiver == receiverWeb && prometheusPort == port {
		return nil, nil, errors.New("Prometheus port and Web receiver port must not be the same")
	}
	c.prometheusPort = uint16(prometheusPort)
	c.exposePrometheus = exposePrometheus

	// Initialize the chosen receiver, tokenizer, aggregator, and forwarder.
	newTokenizer, exists := ourTokenizers[tokenizer]
	if !exists {
		return nil, nil, errors.New("tokenizer does not exist")
	}
	newForwarder, exists := ourForwarders[forwarder]
	if !exists {
		return nil, nil, errors.New("forwarder does not exist")
	}
	newAggregator, exists := ourAggregators[aggregator]
	if !exists {
		return nil, nil, errors.New("aggregator does not exist")
	}
	newReceiver, exists := ourReceivers[receiver]
	if !exists {
		return nil, nil, errors.New("receiver does not exist")
	}
	l.Printf("Using receiver=%s, aggregator=%s, tokenizer=%s, forwarder=%s.",
		receiver, aggregator, tokenizer, forwarder)

	comp := &components{
		a: newAggregator(),
		f: newForwarder(),
		r: newReceiver(),
		t: newTokenizer(),
	}
	return comp, c, nil
}

// exposeMetrics starts an HTTP server at the given port.  The server exposes
// an endpoint for Prometheus metrics.  Note that tokenizer is meant to be run
// inside a Kubernetes pod.  Access to this port is therefore handled by a
// Kubernetes service.  If we are configured to expose Prometheus metrics *and*
// use the Web receiver, we need two Kubernetes services: one that is publicly
// accessible (the Web receiver) and one that's private (the Prometheus
// metrics).
func exposeMetrics(port uint16) {
	http.Handle("/metrics", promhttp.Handler())
	l.Printf("Exposing Prometheus metrics at :%d.", port)
	l.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func main() {
	comp, conf, err := parseFlags(os.Args[0], os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(1)
		}
		l.Fatal(err)
	}
	if conf.exposePrometheus {
		go exposeMetrics(conf.prometheusPort)
	}
	if err := maxSoftFdLimit(); err != nil {
		l.Printf("Failed to maximize soft fd limit: %v", err)
	}
	l.Printf("Config: %+v", conf)
	bootstrap(conf, comp, make(chan empty))
}
