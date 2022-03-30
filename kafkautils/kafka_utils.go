// Package kafkautils provides Kafka utility functions that are shared between
// ia2 and the HTTP-to-Kafka bridge that ia2 relies on if it's running inside
// an enclave.
package kafkautils

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"

	"github.com/segmentio/kafka-go"
)

const (
	// DefaultKafkaKey holds the default path to the Kafka certificate key.
	DefaultKafkaKey = "/etc/kafka/secrets/key"
	// DefaultKafkaCert holds the default path to the Kafka certificate.
	DefaultKafkaCert = "/etc/kafka/secrets/certificate"

	kafkaTestTopic = "antifraud_client_addrs_events.testing.repsys.upstream"
	envKafkaBroker = "KAFKA_BROKERS"
)

var l = log.New(os.Stderr, "kafkautils: ", log.Ldate|log.Ltime|log.LUTC|log.Lshortfile)

// NewKafkaWriter creates a new Kafka writer based on the environment variable
// envKafkaBroker and the given certificate files.
func NewKafkaWriter(certFile, keyFile string) (*kafka.Writer, error) {
	kafkaBroker, exists := os.LookupEnv(envKafkaBroker)
	if !exists {
		return nil, fmt.Errorf("environment variable %q not set", envKafkaBroker)
	}
	if kafkaBroker == "" {
		return nil, fmt.Errorf("environment variable %q empty", envKafkaBroker)
	}
	l.Printf("Fetched Kafka broker %q from environment variable.", kafkaBroker)

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	l.Println("Loaded certificate and key file for Kafka.")

	return &kafka.Writer{
		Addr:  kafka.TCP(kafkaBroker),
		Topic: kafkaTestTopic,
		Transport: &kafka.Transport{
			TLS: &tls.Config{Certificates: []tls.Certificate{cert}},
		},
	}, nil
}
