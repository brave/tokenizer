package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/segmentio/kafka-go"
)

const (
	defaultBatchPeriod = time.Second * 30
	defaultBatchSize   = 1000
	envKafkaClientCert = "KAFKA_CLIENT_CERT"
	envKafkaClientKey  = "KAFKA_CLIENT_KEY"
	envKafkaInterCert  = "KAFKA_INTERMEDIATE_CERT"
	envKafkaInterChain = "KAFKA_INTERMEDIATE_CHAIN"
	envKafkaRootCert   = "KAFKA_ROOT_CERT"
	envKafkaBroker     = "KAFKA_BROKERS"
	envKafkaTopic      = "KAFKA_TOPIC"
	// amazonRootCACert is the certificate of one of Amazon's root CAs.  The
	// certificate chain that we encounter when connecting to our Kafka broker
	// goes up to this CA.  The root certificates are available at:
	// https://www.amazontrust.com/repository/
	amazonRootCACert = `
-----BEGIN CERTIFICATE-----
MIID7zCCAtegAwIBAgIBADANBgkqhkiG9w0BAQsFADCBmDELMAkGA1UEBhMCVVMx
EDAOBgNVBAgTB0FyaXpvbmExEzARBgNVBAcTClNjb3R0c2RhbGUxJTAjBgNVBAoT
HFN0YXJmaWVsZCBUZWNobm9sb2dpZXMsIEluYy4xOzA5BgNVBAMTMlN0YXJmaWVs
ZCBTZXJ2aWNlcyBSb290IENlcnRpZmljYXRlIEF1dGhvcml0eSAtIEcyMB4XDTA5
MDkwMTAwMDAwMFoXDTM3MTIzMTIzNTk1OVowgZgxCzAJBgNVBAYTAlVTMRAwDgYD
VQQIEwdBcml6b25hMRMwEQYDVQQHEwpTY290dHNkYWxlMSUwIwYDVQQKExxTdGFy
ZmllbGQgVGVjaG5vbG9naWVzLCBJbmMuMTswOQYDVQQDEzJTdGFyZmllbGQgU2Vy
dmljZXMgUm9vdCBDZXJ0aWZpY2F0ZSBBdXRob3JpdHkgLSBHMjCCASIwDQYJKoZI
hvcNAQEBBQADggEPADCCAQoCggEBANUMOsQq+U7i9b4Zl1+OiFOxHz/Lz58gE20p
OsgPfTz3a3Y4Y9k2YKibXlwAgLIvWX/2h/klQ4bnaRtSmpDhcePYLQ1Ob/bISdm2
8xpWriu2dBTrz/sm4xq6HZYuajtYlIlHVv8loJNwU4PahHQUw2eeBGg6345AWh1K
Ts9DkTvnVtYAcMtS7nt9rjrnvDH5RfbCYM8TWQIrgMw0R9+53pBlbQLPLJGmpufe
hRhJfGZOozptqbXuNC66DQO4M99H67FrjSXZm86B0UVGMpZwh94CDklDhbZsc7tk
6mFBrMnUVN+HL8cisibMn1lUaJ/8viovxFUcdUBgF4UCVTmLfwUCAwEAAaNCMEAw
DwYDVR0TAQH/BAUwAwEB/zAOBgNVHQ8BAf8EBAMCAQYwHQYDVR0OBBYEFJxfAN+q
AdcwKziIorhtSpzyEZGDMA0GCSqGSIb3DQEBCwUAA4IBAQBLNqaEd2ndOxmfZyMI
bw5hyf2E3F/YNoHN2BtBLZ9g3ccaaNnRbobhiCPPE95Dz+I0swSdHynVv/heyNXB
ve6SbzJ08pGCL72CQnqtKrcgfU28elUSwhXqvfdqlS5sdJ/PHLTyxQGjhdByPq1z
qwubdQxtRbeOlKyWN7Wg0I8VRw7j6IPdj/3vQQF3zCepYoUz8jcI73HPdwbeyBkd
iEDPfUYd/x7H4c7/I9vG+o1VTqkC50cRRj70/b17KSa7qWFiNyi2LSr2EIZkyXCn
0q23KXB56jzaYyWf/Wi3MOxw+3WKt21gZ7IeyLnp2KhvAotnDU0mV3HaIPzBSlCN
sSi6
-----END CERTIFICATE-----`
)

var (
	errEnvVarUnset  = errors.New("environment variable unset")
	errNothingToFwd = errors.New("nothing to forward")
)

// kafkaWriter defines an interface that's implemented by kafka-go's
// kafka.Writer (which we use in production) and by dummyKafkaWriter (which we
// use for tests).
type kafkaWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
}

type kafkaConfig struct {
	batchPeriod time.Duration
	batchSize   int
	clientCert  *tls.Certificate
	serverCerts *x509.CertPool
	broker      net.Addr
	topic       string
}

// kafkaForwarder implements a forwarder that sends tokenized data to a Kafka
// broker.
type kafkaForwarder struct {
	sync.RWMutex
	msgBatch  []kafka.Message
	lastBatch time.Time
	conf      *kafkaConfig
	writer    kafkaWriter
	out       chan token
	done      chan empty
}

func newKafkaForwarder() forwarder {
	return &kafkaForwarder{
		msgBatch:  []kafka.Message{},
		lastBatch: time.Now(),
		out:       make(chan token),
		done:      make(chan empty),
	}
}

func (k *kafkaForwarder) setConfig(c *config) {
	k.Lock()
	defer k.Unlock()

	k.conf = c.kafkaConfig
}

func (k *kafkaForwarder) outbox() chan token {
	return k.out
}

func (k *kafkaForwarder) start() {
	k.Lock()
	k.writer = newKafkaWriter(k.conf)
	k.Unlock()

	go func() {
		for {
			select {
			case <-k.done:
				return
			case token := <-k.out:
				if err := k.send(token); err != nil {
					l.Printf("Failed to send token: %v", err)
				}
			}
		}
	}()
}

func (k *kafkaForwarder) stop() {
	close(k.done)
}

func (k *kafkaForwarder) canBatchAge() bool {
	return time.Now().Add(-k.conf.batchPeriod).Before(k.lastBatch)
}

func (k *kafkaForwarder) canBatchGrow() bool {
	return len(k.msgBatch) < k.conf.batchSize
}

func (k *kafkaForwarder) resetBatch() {
	k.lastBatch = time.Now()
	k.msgBatch = []kafka.Message{}
}

func (k *kafkaForwarder) send(t token) error {
	k.Lock()
	defer k.Unlock()

	if len(t) == 0 {
		m.numForwarded.With(prometheus.Labels{outcome: failBecause(errNothingToFwd)}).Inc()
		return errNothingToFwd
	}

	// We batch messages until 1) the batch gets too large or 2) the batch gets
	// too old -- whichever comes first.
	if k.canBatchAge() && k.canBatchGrow() {
		k.msgBatch = append(k.msgBatch, kafka.Message{
			Key:   nil,
			Value: t,
		})
		return nil
	}

	err := k.writer.WriteMessages(context.Background(), k.msgBatch...)
	if err != nil {
		err := fmt.Errorf("failed to forward blob to Kafka: %w", err)
		m.numForwarded.With(prometheus.Labels{outcome: failBecause(err)}).Inc()
		return err
	}
	l.Printf("Sent %d tokens to Kafka.", len(k.msgBatch))
	k.resetBatch()
	m.numForwarded.With(prometheus.Labels{outcome: success}).Inc()
	return nil
}

func newKafkaWriter(conf *kafkaConfig) *kafka.Writer {
	w := &kafka.Writer{
		Addr:  conf.broker,
		Topic: conf.topic,
		Transport: &kafka.Transport{
			TLS: &tls.Config{
				Certificates: []tls.Certificate{*conf.clientCert},
				// As of 2022-12-21, our Kafka broker does not support TLS 1.3,
				// which is why we're enforcing at least 1.2.
				MinVersion: tls.VersionTLS12,
				RootCAs:    conf.serverCerts,
			},
		},
	}
	l.Printf("Created Kafka writer for %q using topic %q.", conf.broker, conf.topic)
	return w
}

func loadKafkaCerts() (*tls.Certificate, *x509.CertPool, error) {
	clientCertPath, exists := os.LookupEnv(envKafkaClientCert)
	if !exists {
		return nil, nil, errEnvVarUnset
	}
	clientKeyPath, exists := os.LookupEnv(envKafkaClientKey)
	if !exists {
		return nil, nil, errEnvVarUnset
	}
	clientCert, err := loadKafkaClientCert(clientCertPath, clientKeyPath)
	if err != nil {
		return nil, nil, err
	}

	interCertPath, exists := os.LookupEnv(envKafkaInterCert)
	if !exists {
		return nil, nil, errEnvVarUnset
	}
	interChainPath, exists := os.LookupEnv(envKafkaInterChain)
	if !exists {
		return nil, nil, errEnvVarUnset
	}
	rootCertPath, exists := os.LookupEnv(envKafkaRootCert)
	if !exists {
		return nil, nil, errEnvVarUnset
	}
	serverCerts, err := loadKafkaServerCerts(
		[]string{interCertPath, interChainPath, rootCertPath},
	)
	if err != nil {
		return nil, nil, err
	}

	return clientCert, serverCerts, nil
}

func loadKafkaClientCert(cert, key string) (*tls.Certificate, error) {
	clientCert, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}
	return &clientCert, nil
}

func loadKafkaServerCerts(paths []string) (*x509.CertPool, error) {
	ourRootCAs, err := x509.SystemCertPool()
	if err != nil {
		l.Printf("Failed to instantiate system cert pool: %v", err)
		ourRootCAs = x509.NewCertPool()
	}

	for _, path := range paths {
		c, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if ok := ourRootCAs.AppendCertsFromPEM(c); !ok {
			return nil, errors.New("failed to parse intermediate certificate")
		}
	}
	if ok := ourRootCAs.AppendCertsFromPEM([]byte(amazonRootCACert)); !ok {
		return nil, errors.New("failed to parse Amazon's root CA certificate")
	}

	return ourRootCAs, nil
}

func loadKafkaConfig() (*kafkaConfig, error) {
	clientCert, serverCerts, err := loadKafkaCerts()
	if err != nil {
		return nil, err
	}

	broker, exists := os.LookupEnv(envKafkaBroker)
	if !exists {
		return nil, errEnvVarUnset
	}
	// If we're dealing with a comma-separated list of brokers, simply select
	// the first one.
	broker = strings.Split(broker, ",")[0]

	topic, exists := os.LookupEnv(envKafkaTopic)
	if !exists {
		return nil, errEnvVarUnset
	}

	l.Println("Loaded Kafka config.")
	return &kafkaConfig{
		batchSize:   defaultBatchSize,
		batchPeriod: defaultBatchPeriod,
		clientCert:  clientCert,
		serverCerts: serverCerts,
		broker:      kafka.TCP(broker),
		topic:       topic,
	}, nil
}
