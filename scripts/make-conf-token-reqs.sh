#!/bin/bash
#
# This script helps with stress-testing tokenizer's Web receiver.

if [ "$#" -ne 1 ]; then
    >&2 echo "Usage: $0 URL"
    exit 1
fi
url="$1"

ok=200
# Keep track of (un)successful backend responses.
num_good=0
num_bad=0

echo "Making requests to ${url}.  Press Ctrl-C to stop."
while true; do
    # Generate random IPv4 address.
    addr=$(printf "%d.%d.%d.%d\n" \
           "$((RANDOM % 256))" \
           "$((RANDOM % 256))" \
           "$((RANDOM % 256))" \
           "$((RANDOM % 256))")
    resp_code=$(
        curl \
            --silent \
            --write-out "%{response_code}\n" \
            --header "Fastly-Client-IP: ${addr}" \
            "${url}/v3/confirmation/token/$(uuidgen)"
    )

    if [ "$resp_code" -eq "$ok" ]; then
        num_good=$((num_good+1))
    else
        num_bad=$((num_bad+1))
    fi
    echo -en "\r${num_good} successful, ${num_bad} failed backend responses."
    sleep 0.1
done
