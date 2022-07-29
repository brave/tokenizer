package kafkautils

import (
	"errors"
	"os"
	"testing"
)

func TestLookupEnv(t *testing.T) {
	envVar := "FOO"
	envValue := "BAR"
	if err := os.Setenv(envVar, envValue); err != nil {
		t.Fatalf("Failed to set env var %q: %s", envVar, err)
	}

	maybeValue, err := lookupEnv(envVar)
	if err != nil {
		t.Fatalf("Failed to get env var %q: %s", envVar, err)
	}
	if maybeValue != envValue {
		t.Fatalf("Expected env var to be %q but got %q.", envValue, maybeValue)
	}

	// Now try to retrieve a non-existing environment variable.
	if err := os.Unsetenv(envVar); err != nil {
		t.Fatalf("Failed to unset env var %q: %s", envVar, err)
	}
	if _, err := lookupEnv(envVar); err == nil {
		t.Fatalf("Expected error when looking up unset env var.")
	}
}

func TestNewKafkaWriter(t *testing.T) {
	var err error

	// Make sure that our environment variable is unset.
	if err := os.Unsetenv(envKafkaBroker); err != nil {
		t.Fatalf("Failed to unset env variable: %s", err)
	}

	_, err = NewKafkaWriter(DefaultKafkaCert, DefaultKafkaKey, DefaultKafkaCACert)
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
	_, err = NewKafkaWriter(DefaultKafkaCert, DefaultKafkaKey, DefaultKafkaCACert)
	if err == nil {
		t.Fatal("Failed to return an error because cert doesn't exist.")
	}
}
