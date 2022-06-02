IP Address Anonymizer (ia2)
===========================

ia2 anonymizes IP addresses.  The service takes as input IP addresses via an
HTTP API.  Those addresses are then anonymized and forwarded to a Kafka cluster.
ia2 is meant to (but doesn't have to) run in an [AWS Nitro
Enclave](https://aws.amazon.com/ec2/nitro/nitro-enclaves/) and exposes the
following two HTTP API endpoints:

1. `/v2/confirmation/token/WALLET_ID`  
  This endpoint takes as input confirmation token requests as received by
  Fastly.  The endpoint extracts the client IP address, anonymizes it, and
  forwards it to our Kafka cluster.

2. `/attestation`  
  This endpoint handles requests for remote attestation.  Clients provide a
  nonce, which is then forwarded to the Nitro hypervisor, which returns an
  attestation document, allowing clients to verify the authenticity of the
  secure enclave.

Developer setup
---------------

To test, lint, and compile ia2, run:

    make

You can then start ia2 by executing the `ia2` binary.  Note that you don't need
to run ia2 inside a Nitro Enclave: the code (in particular the
[nitriding](https://github.com/brave-experiments/nitriding)
package that ia2 depends on) checks if it's inside an enclave and if not, it
skips enclave-specific setup to facilitate local development.

To create a reproducible Docker image of ia2, run:

    make docker

Configuration
-------------

ia2 is not meant to be run interactively, and is therefore configured via
constants in [main.go](main.go).  All of those constants are documented, so
please refer to the source code to learn more about configuration options.

Architecture
------------

To learn more about ia2's architecture, take a look at the [architectural
documentation](doc/architecture.md).
