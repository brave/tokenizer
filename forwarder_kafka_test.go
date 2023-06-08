package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"testing"

	"github.com/segmentio/kafka-go"
)

// The following contains dummy certificates that we use for testing only.
var (
	clientCert = []byte(`
-----BEGIN CERTIFICATE-----
MIIBszCCAVkCFF9AFyJZRH58rka3xkDi0PNmSIALMAoGCCqGSM49BAMCMFsxCzAJ
BgNVBAYTAlVTMRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5l
dCBXaWRnaXRzIFB0eSBMdGQxFDASBgNVBAMMC2V4YW1wbGUuY29tMCAXDTIzMDEw
OTE0MDM0OVoYDzIyOTYxMDI0MTQwMzQ5WjBbMQswCQYDVQQGEwJVUzETMBEGA1UE
CAwKU29tZS1TdGF0ZTEhMB8GA1UECgwYSW50ZXJuZXQgV2lkZ2l0cyBQdHkgTHRk
MRQwEgYDVQQDDAtleGFtcGxlLmNvbTBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IA
BMZc3cPzhei6gV28557eYLDnYiwRBF/dl/XS5eCiFrlGH6jQTLFbGRNOvxuxLklY
Z3lcsZetJspmTxAeYO80isYwCgYIKoZIzj0EAwIDSAAwRQIhALhebyZs4oNu7NWX
k9tAAR6ioZ0HoQ77iqQXjok8t5JrAiA5B3/nRDORNa7kvJCuReMbc6T+z3Zw9kIA
P7io+r7CFA==
-----END CERTIFICATE-----
`)
	clientKey = []byte(`
-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIIScaJV2GYTk+UJRNDh1y96wS627Ik83FMuzvo9Zw7CZoAoGCCqGSM49
AwEHoUQDQgAExlzdw/OF6LqBXbznnt5gsOdiLBEEX92X9dLl4KIWuUYfqNBMsVsZ
E06/G7EuSVhneVyxl60mymZPEB5g7zSKxg==
-----END EC PRIVATE KEY-----
`)
	caCert = []byte(`
-----BEGIN CERTIFICATE-----
MIICCzCCAbGgAwIBAgIUEYnVajHs5EBQDTwGf5zthQAHjsIwCgYIKoZIzj0EAwIw
WzELMAkGA1UEBhMCVVMxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoMGElu
dGVybmV0IFdpZGdpdHMgUHR5IEx0ZDEUMBIGA1UEAwwLZXhhbXBsZS5jb20wHhcN
MjMwMTA5MTQwMDQzWhcNMjMwMjA4MTQwMDQzWjBbMQswCQYDVQQGEwJVUzETMBEG
A1UECAwKU29tZS1TdGF0ZTEhMB8GA1UECgwYSW50ZXJuZXQgV2lkZ2l0cyBQdHkg
THRkMRQwEgYDVQQDDAtleGFtcGxlLmNvbTBZMBMGByqGSM49AgEGCCqGSM49AwEH
A0IABFGM1lvl0dzSwNvI/jlEZ7jpa/FExC4NLe8ioEkvWnXagwo+b1lLUzKfULUG
zu0amYavYDjRqGcDJ/frDKE/+dujUzBRMB0GA1UdDgQWBBTfhICiIPL4IVCmd9A/
rqaG6M2a5DAfBgNVHSMEGDAWgBTfhICiIPL4IVCmd9A/rqaG6M2a5DAPBgNVHRMB
Af8EBTADAQH/MAoGCCqGSM49BAMCA0gAMEUCIQChy6R/fA6ePm6DIcFABueIbl1W
DGGv8fpHLT0qXYQOaAIgcATqqJSvvK81W6YdqLGDGqf6l+BX9CdwDWh/tRISDeI=
-----END CERTIFICATE-----
`)
)

type dummyKafkaWriter struct{}

func (d *dummyKafkaWriter) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	return nil
}

func createKafkaConf(t *testing.T) *kafkaConfig {
	clientKeyPair, err := tls.X509KeyPair(clientCert, clientKey)
	if err != nil {
		t.Fatalf("Failed to load client key pair: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caCert)

	return &kafkaConfig{
		clientCert:  &clientKeyPair,
		serverCerts: pool,
		broker:      kafka.TCP("example.com"),
		topic:       "foobar",
	}
}

func writeFile(t *testing.T, content []byte, name string) string {
	f, err := os.CreateTemp("", name)
	if err != nil {
		t.Fatalf("Failed to create temporary file: %v", err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatalf("Failed to write to temporary file: %v", err)
	}
	return f.Name()
}

func TestLoadKafkaConfig(t *testing.T) {
	pathClientCert := writeFile(t, clientCert, "client.crt")
	defer os.Remove(pathClientCert)
	pathClientKey := writeFile(t, clientKey, "client.key")
	defer os.Remove(pathClientKey)
	pathRootCert := writeFile(t, caCert, "ca.crt")
	defer os.Remove(pathRootCert)

	envToValue := map[string]string{
		envKafkaClientCert: pathClientCert,
		envKafkaClientKey:  pathClientKey,
		envKafkaRootCert:   pathRootCert,
		envKafkaInterCert:  pathRootCert, // Re-use root cert.
		envKafkaInterChain: pathRootCert, // Re-use root cert.
		envKafkaBroker:     "foo",
		envKafkaTopic:      "bar",
	}
	for env, path := range envToValue {
		if err := os.Setenv(env, path); err != nil {
			t.Fatalf("Failed to set environment variable: %v", err)
		}
	}
	_, err := loadKafkaConfig()
	if err != nil {
		t.Fatalf("Failed to load Kafka certificates: %v", err)
	}

}

func TestSend(t *testing.T) {
	maxBatchSize := 2
	k := newKafkaForwarder().(*kafkaForwarder)
	k.writer = &dummyKafkaWriter{}
	k.setConfig(&config{
		kafkaConfig: &kafkaConfig{
			batchPeriod: defaultBatchPeriod,
			batchSize:   maxBatchSize,
		},
	})
	origBatchAge := k.lastBatch

	assertEqual(t, k.send(token([]byte("foo"))), nil)
	assertEqual(t, k.send(token([]byte("bar"))), nil)
	assertEqual(t, len(k.msgBatch), maxBatchSize)
	assertEqual(t, k.send(token([]byte("baz"))), nil)
	assertEqual(t, len(k.msgBatch), 0)
	assertEqual(t, origBatchAge != k.lastBatch, true)
}
