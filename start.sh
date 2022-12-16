#!/bin/sh

set -x

# Invoke script that takes care of Kafka certificates.
entrypoint="/bin/entrypoint.sh"
if [ -f "$entrypoint" ]
then
    /bin/entrypoint.sh &
fi

# Start ia2, our main service.
/ia2
