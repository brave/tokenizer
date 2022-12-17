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
	// KafkaInterCert holds the path to our Kafka intermediate cert.
	KafkaInterCert = "/etc/ssl/cacerts/intermediate_certificate"
	// KafkaInterChain holds the path to our Kafka intermediate cert chain.
	KafkaInterChain = "/etc/ssl/cacerts/intermediate_certificate_chain"
	// KafkaRootCert holds the path to our Kafka root cert.
	KafkaRootCert  = "/etc/ssl/cacerts/root_certificate"
	envKafkaBroker = "KAFKA_BROKERS"
	envKafkaTopic  = "KAFKA_TOPIC"
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
func NewKafkaWriter(certFile, keyFile string) (*kafka.Writer, error) {
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

	// Client certificate.
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	// Server certificate chain.
	rawInterCert, err := os.ReadFile(KafkaInterCert)
	if err != nil {
		return nil, err
	}
	rawInterChain, err := os.ReadFile(KafkaInterChain)
	if err != nil {
		return nil, err
	}
	rawRootCert, err := os.ReadFile(KafkaRootCert)
	if err != nil {
		return nil, err
	}
	l.Println("Loaded client and server certificates from files.")

	ourRootCAs, err := x509.SystemCertPool()
	if err != nil {
		l.Printf("Failed to instantiate system cert pool: %s", err)
		ourRootCAs = x509.NewCertPool()
	}
	if ok := ourRootCAs.AppendCertsFromPEM(rawInterCert); !ok {
		return nil, errors.New("failed to parse intermediate certificate")
	}
	if ok := ourRootCAs.AppendCertsFromPEM(rawInterChain); !ok {
		return nil, errors.New("failed to parse intermediate certificate chain")
	}
	if ok := ourRootCAs.AppendCertsFromPEM(rawRootCert); !ok {
		return nil, errors.New("failed to parse root certificate")
	}
	if ok := ourRootCAs.AppendCertsFromPEM([]byte(amazonRootCACert)); !ok {
		return nil, errors.New("failed to parse Amazon root certificate")
	}

	l.Printf("Creating Kafka writer for %q using topic %q.", kafkaBroker, kafkaTopic)
	return &kafka.Writer{
		Addr:  kafka.TCP(kafkaBroker),
		Topic: kafkaTopic,
		Transport: &kafka.Transport{
			TLS: &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
				RootCAs:      ourRootCAs,
			},
		},
	}, nil
}
