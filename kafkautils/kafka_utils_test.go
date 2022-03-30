package kafkautils

import (
	"errors"
	"os"
	"testing"
)

func TestNewKafkaWriter(t *testing.T) {
	var err error

	// Make sure that our environment variable is unset.
	if err := os.Unsetenv(envKafkaBroker); err != nil {
		t.Fatalf("Failed to unset env variable: %s", err)
	}

	_, err = NewKafkaWriter(DefaultKafkaCert, DefaultKafkaKey)
	if err == nil {
		t.Fatal("Failed to return an error because env variable was unset.")
	}

	// Abort if our Kafka certificate file exists.
	if _, err := os.Stat(DefaultKafkaCert); !errors.Is(err, os.ErrNotExist) {
		return
	}
	if err := os.Setenv(envKafkaBroker, "127.0.0.1:1234"); err != nil {
		t.Fatalf("Failed to set env variable: %s", err)
	}
	_, err = NewKafkaWriter(DefaultKafkaCert, DefaultKafkaKey)
	if err == nil {
		t.Fatal("Failed to return an error because cert doesn't exist.")
	}
}
