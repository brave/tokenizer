// Package kafkautils provides Kafka utility functions that are shared between
// ia2 and the HTTP-to-Kafka bridge that ia2 relies on if it's running inside
// an enclave.
package kafkautils

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/segmentio/kafka-go"
)

const (
	// DefaultKafkaKey holds the default path to the Kafka certificate key.
	DefaultKafkaKey = "/etc/kafka/secrets/key"
	// DefaultKafkaCert holds the default path to the Kafka certificate.
	DefaultKafkaCert = "/etc/kafka/secrets/certificate"
	// DefaultKafkaCAChain holds the default path to our Kafka root CA certificate chain.
	DefaultKafkaCAChain = "/etc/ssl/cacerts/certificate_chain"
	// DefaultKafkaCACert holds the default path to our Kafka root CA certificate.
	DefaultKafkaCACert = "/etc/ssl/cacerts/certificate"
	envKafkaBroker     = "KAFKA_BROKERS"
	envKafkaTopic      = "KAFKA_TOPIC"
)

var l = log.New(os.Stderr, "kafkautils: ", log.Ldate|log.Ltime|log.LUTC|log.Lshortfile)

func lookupEnv(envVar string) (string, error) {
	value, exists := os.LookupEnv(envVar)
	if !exists {
		return "", fmt.Errorf("environment variable %q not set", envVar)
	}
	if value == "" {
		return "", fmt.Errorf("environment variable %q empty", envVar)
	}
	return value, nil
}

// NewKafkaWriter creates a new Kafka writer based on the environment variable
// envKafkaBroker and the given certificate files.
func NewKafkaWriter(certFile, keyFile, caFile string) (*kafka.Writer, error) {
	kafkaBrokers, err := lookupEnv(envKafkaBroker)
	if err != nil {
		return nil, err
	}
	// If we're dealing with a comma-separated list of brokers, simply select
	// the first one.
	kafkaBroker := strings.Split(kafkaBrokers, ",")[0]

	l.Printf("Fetched Kafka broker %q from environment variable.", kafkaBroker)
	kafkaTopic, err := lookupEnv(envKafkaTopic)
	if err != nil {
		return nil, err
	}
	l.Printf("Fetched Kafka topic %q from environment variable.", kafkaTopic)

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	l.Println("Loaded certificate and key file for Kafka.")

	rawChain, err := os.ReadFile(DefaultKafkaCAChain)
	if err != nil {
		return nil, err
	}
	l.Printf("CA certificate chain:\n%s\n", rawChain)

	rawCert, err := os.ReadFile(DefaultKafkaCACert)
	if err != nil {
		return nil, err
	}
	l.Printf("CA certificate:\n%s\n", rawCert)

	ourRootCAs := x509.NewCertPool()
	if ok := ourRootCAs.AppendCertsFromPEM([]byte(rawChain)); !ok {
		return nil, errors.New("failed to parse root certificate chain")
	}
	if ok := ourRootCAs.AppendCertsFromPEM([]byte(rawCert)); !ok {
		return nil, errors.New("failed to parse root certificate")
	}

	return &kafka.Writer{
		Addr:  kafka.TCP(kafkaBroker),
		Topic: kafkaTopic,
		Transport: &kafka.Transport{
			TLS: &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS13,
				RootCAs:      ourRootCAs,
			},
		},
	}, nil
}
