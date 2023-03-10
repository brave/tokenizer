package main

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func assertEqual(t *testing.T, is, should interface{}) {
	t.Helper()
	if should != is {
		t.Fatalf("Expected value %v but got %v.", should, is)
	}
}

func TestParseFlags(t *testing.T) {
	// This test was inspired by Eli Bendersky's 2020 blog post:
	// https://eli.thegreenplace.net/2020/testing-flag-parsing-in-go-programs/
	tests := []struct {
		args []string
		conf *config
	}{
		{
			[]string{"-forward-interval", "1", "-key-expiry", "2", "-port", "80"},
			&config{
				fwdInterval:    time.Second,
				keyExpiry:      time.Second * 2,
				port:           80,
				prometheusPort: 8081,
			},
		},
	}

	for _, test := range tests {
		t.Run(
			strings.Join(test.args, " "),
			func(t *testing.T) {
				_, conf, err := parseFlags("tkzr", test.args)
				if err != nil {
					t.Fatalf("Got unexpected error: %v", err)
				}
				if !reflect.DeepEqual(conf, test.conf) {
					t.Fatalf("Expected conf %+v but got %+v.", test.conf, conf)
				}
			},
		)
	}
}

func TestBootstrap(t *testing.T) {
	done := make(chan empty)
	go func() {
		bootstrap(
			&config{},
			&components{
				a: newSimpleAggregator(),
				r: newStdinReceiver(),
				f: newStdoutForwarder(),
				t: newVerbatimTokenizer(),
			},
			done,
		)
	}()
	close(done)
}
