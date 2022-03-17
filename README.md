IP Address Anonymizer (ia2)
===========================

This service is meant to be run in an AWS Nitro enclave and exposes an HTTP API
with two endpoints:

1. One endpoint takes as input IP addresses.  The service then anonymizes those
   IP addresses and sends them to a Kafka broker.

2. The other endpoint lets clients request an attestation document (issued by
   the Nitro hypervisor) that lets clients attest the enclave's authenticity.

Developer setup
---------------

Simply run `make` to test and compile the service.  You can then start ia2 by
running `./ia2`.  Note that the code checks if it's being run inside an enclave.
If it's not in an enclave, ia2 skips enclave-specific setup, so it can be run
locally.

Configuration
-------------

ia2 is not meant to be run interactively, and is therefore configured via
constants in [main.go](main.go).  All of those constants are documented, so
please refer to the source code to learn more about configuration options.
