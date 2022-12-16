#!/bin/bash

set -x

# Invoke script that takes care of Kafka certificates.
entrypoint="/bin/entrypoint.sh"
if [ -f "$entrypoint" ]
then
    /bin/entrypoint.sh &
    sleep 5
fi

# Start ia2, our main service.
/bin/ia2
