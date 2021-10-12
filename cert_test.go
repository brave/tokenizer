package main

import (
	"crypto/x509"
	"testing"
)

func TestGenSelfSignedCert(t *testing.T) {
	fqdn := "foo.bar"
	rawCert, err := genSelfSignedCert(fqdn)
	if err != nil {
		t.Fatal(err)
	}

	cert, err := x509.ParseCertificate(rawCert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}

	if cert.Subject.Organization[0] != certificateOrg {
		t.Fatalf("Expected organization %q but got %q.", certificateOrg, cert.Subject.Organization)
	}

	if err = cert.VerifyHostname(fqdn); err != nil {
		t.Fatal(err)
	}
}
