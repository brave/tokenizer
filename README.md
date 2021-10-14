IP Address Anonymizer (ia2)
===========================

This service is meant to be run in an AWS Nitro enclave and exposes an HTTP API
with two endpoints:

1. One endpoint takes as input IP addresses.  The service then anonymizes those
   IP addresses and sends them to a Kafka broker.

2. The other endpoint lets clients request an attestation document (issued by
   the Nitro hypervisor) that lets clients attest the enclave's authenticity.
