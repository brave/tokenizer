package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log"
	"math/big"
	"time"
)

const (
	certificateOrg      = "Brave Software"
	certificateValidity = time.Hour * 24 * 356
)

// genSelfSignedCert creates and returns a self-signed TLS certificate based on
// the given FQDN.  Some of the code below was taken from:
// https://eli.thegreenplace.net/2021/go-https-servers-with-tls/
func genSelfSignedCert(fqdn string) (tls.Certificate, error) {

	log.Println("Generating private key for certificate.")
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate private key: %v", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		log.Fatalf("Failed to generate serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{certificateOrg},
		},
		DNSNames:              []string{fqdn},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(certificateValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	log.Println("Generating certificate from template.")
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		log.Fatalf("Failed to create certificate: %v", err)
	}

	pemCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	if pemCert == nil {
		log.Fatal("Failed to encode certificate to PEM.")
	}
	// Determine and set the certificate's fingerprint because we need to add
	// the fingerprint to our Nitro attestation document.
	setCertFingerprint(pemCert)

	privBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		log.Fatalf("Unable to marshal private key: %v", err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
	if pemKey == nil {
		log.Fatal("Failed to encode key to PEM.")
	}

	return tls.X509KeyPair(pemCert, pemKey)
}
