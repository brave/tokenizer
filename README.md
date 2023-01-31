# Tokenizer

Tokenizer receives sensitive input from somewhere, tokenizes it, and sends the
output somewhere else.  This summary is deliberately vague because tokenizer's
input, output, and tokenization are pluggable: Input can come from an HTTP API
or stdin.  Tokenization can be done by a
[HMAC-SHA256](https://en.wikipedia.org/wiki/HMAC)
or
[CryptoPAn](https://en.wikipedia.org/wiki/Crypto-PAn).
The output can be a Kafka broker or stdout.  Tokenizer further supports
pluggable aggregation, which dictates how input is processed.

# Development

To test, lint, and build `tkzr`, run:

    make

To create a reproducible Docker image, run:

    make docker

To understand tokenizer's architecture, begin by studying its
[interfaces](interfaces.go).

# Usage

Run the following command to compile `tkzr`.

    make tkzr

Use the following command to run tokenizer with the given receiver, forwarder,
and tokenizer:

    tkzr -receiver stdin -tokenizer hmac -forwarder stdout
