package main

import (
	"errors"
	"log"

	"github.com/hf/nsm"
	"github.com/hf/nsm/request"
)

// attest takes as input a nonce, user-provided data and a public key, and then
// asks the Nitro hypervisor to return a signed attestation document that
// contains all three values.
func attest(nonce, userData, publicKey []byte) ([]byte, error) {
	s, err := nsm.OpenDefaultSession()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err = s.Close(); err != nil {
			log.Printf("Failed to close default NSM session: %s", err)
		}
	}()

	// We ignore the error because of a bug that will return an error despite
	// having obtained an attestation document:
	// https://github.com/hf/nsm/issues/2
	res, _ := s.Send(&request.Attestation{
		Nonce:     nonce,
		UserData:  userData,
		PublicKey: []byte{},
	})
	if res.Error != "" {
		return nil, errors.New(string(res.Error))
	}

	if res.Attestation == nil || res.Attestation.Document == nil {
		return nil, errors.New("NSM device did not return an attestation")
	}

	return res.Attestation.Document, nil
}
