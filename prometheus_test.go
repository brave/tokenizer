package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetrics(t *testing.T) {
	done := make(chan empty)
	rc := newWebReceiver()
	tk := newCryptoPAnTokenizer()

	go func() {
		bootstrap(
			&config{fwdInterval: time.Second, keyExpiry: time.Second},
			&components{
				a: newAddrAggregator(),
				f: newStdoutForwarder(),
				r: rc,
				t: tk,
			},
			done,
		)
	}()
	defer close(done)

	// Prepare to make HTTP requests to our HTTP API.
	path := fmt.Sprintf("/v2/confirmation/token/%s", newV4(t))
	srv := httptest.NewServer(rc.(*webReceiver).router)
	defer srv.Close()

	// Make a valid request.
	resp := makeReq(t, srv, http.MethodGet, path, http.Header{fastlyClientIP: []string{"1.1.1.1"}})
	assertEqual(t, resp.StatusCode, http.StatusOK)

	// Make another valid request, but with a different IP address.
	resp = makeReq(t, srv, http.MethodGet, path, http.Header{fastlyClientIP: []string{"2.2.2.2"}})
	assertEqual(t, resp.StatusCode, http.StatusOK)

	// Now, make an invalid request.
	resp = makeReq(t, srv, http.MethodGet, path, http.Header{fastlyClientIP: []string{"foobar"}})
	assertEqual(t, resp.StatusCode, http.StatusBadRequest)

	// Make another invalid request, but without the Fastly header.
	resp = makeReq(t, srv, http.MethodGet, path, http.Header{})
	assertEqual(t, resp.StatusCode, http.StatusBadRequest)

	// Make sure that we collected all HTTP responses by code and body.
	labels := m.webResponses.WithLabelValues
	assertEqual(t, testutil.ToFloat64(labels("200", "")), float64(2))
	assertEqual(t, testutil.ToFloat64(labels("400", errBadFastlyAddrFormat.Error())), float64(1))
	assertEqual(t, testutil.ToFloat64(labels("400", errNoFastlyHeader.Error())), float64(1))

	// Make sure that the total number of metrics (which is different from the
	// total number of HTTP requests) is correct.
	assertEqual(t, testutil.CollectAndCount(m.webResponses), 3)

	// Verify the aggregator's metrics.
	assertEqual(t, testutil.ToFloat64(m.numAddrs), float64(2))
	assertEqual(t, testutil.ToFloat64(m.numWallets), float64(1))

	// Verify the tokenizer's metric.
	labels = m.numTokenized.WithLabelValues
	assertEqual(t, testutil.ToFloat64(labels(success)), float64(2))
	assertEqual(t, testutil.ToFloat64(labels(failBecause(errBadBlobLen))), float64(0))

	// Shove an invalid IP address into the tokenizer and make sure that the
	// metrics got updated accordingly.
	_, err := tk.tokenize(blob("foobar"))
	assertEqual(t, err, errBadBlobLen)

	assertEqual(t, testutil.ToFloat64(labels(success)), float64(2))
	assertEqual(t, testutil.ToFloat64(labels(failBecause(errBadBlobLen))), float64(1))
}

func TestFailBecause(t *testing.T) {
	s := "foo"
	assertEqual(t, failBecause(errors.New(s)), "fail (foo)")
}
