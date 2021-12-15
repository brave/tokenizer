package main

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func expect(t *testing.T, resp *http.Response, statusCode int, errMsg error) {
	if resp.StatusCode != statusCode {
		t.Fatalf("expected status code %d but got %d", statusCode, resp.StatusCode)
	}
	if errMsg == nil {
		return
	}
	payload, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read HTTP response body: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("failed to close response body: %v", err)
		}
	}()
	if strings.TrimSuffix(string(payload), "\n") != errMsg.Error() {
		t.Fatalf("expected error %q but got %q", errMsg.Error(), string(payload))
	}
}

func testReq(t *testing.T, req *http.Request, statusCode int, errMsg error) {
	router := chi.NewRouter()
	router.Post("/address", addressHandler)
	router.Get("/v1/confirmation/token/{walletID}", confTokenHandler)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	expect(t, rec.Result(), statusCode, errMsg)
}

func TestConfTokenHandler(t *testing.T) {
	hmacKey = make([]byte, hmacKeySize)

	testReq(t,
		httptest.NewRequest(http.MethodGet, "/v1/confirmation/token/broken-wallet", nil),
		http.StatusBadRequest,
		errBadWalletFmt,
	)

	testReq(t,
		httptest.NewRequest(http.MethodGet, "/v1/confirmation/token/315c140b-3ae3-4300-a8a1-daf7b008ccb2", nil),
		http.StatusBadRequest,
		errNoFastlyHeader,
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/confirmation/token/315c140b-3ae3-4300-a8a1-daf7b008ccb2", nil)
	req.Header.Set(fastlyClientIP, "badIpAddr")
	testReq(t, req, http.StatusBadRequest, errBadFastlyAddrFormat)

	req.Header.Set(fastlyClientIP, "1.2.3.4")
	testReq(t, req, http.StatusOK, nil)

	req.Header.Set(fastlyClientIP, "::1")
	testReq(t, req, http.StatusOK, nil)
}

func TestAddressHandler(t *testing.T) {
	hmacKey = make([]byte, hmacKeySize)

	testReq(t,
		httptest.NewRequest(http.MethodGet, "/address", nil),
		http.StatusMethodNotAllowed,
		nil,
	)

	testReq(t,
		httptest.NewRequest(http.MethodPost, "/address", nil),
		http.StatusBadRequest,
		errNoAddr,
	)

	req := httptest.NewRequest(http.MethodPost, "/address", bytes.NewReader([]byte("addr=foobar")))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	testReq(t,
		req,
		http.StatusBadRequest,
		errBadAddrFormat,
	)

	req = httptest.NewRequest(http.MethodPost, "/address", bytes.NewReader([]byte("addr=1.2.3.4")))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	testReq(t,
		req,
		http.StatusOK,
		nil,
	)

	req = httptest.NewRequest(http.MethodPost, "/address", bytes.NewReader([]byte("addr=::1")))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	testReq(t,
		req,
		http.StatusOK,
		nil,
	)
}
