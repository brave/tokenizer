#!/bin/bash

ia2_image=$(ko publish --local . 2>/dev/null)

cat > Dockerfile <<EOF
FROM public.ecr.aws/amazonlinux/amazonlinux:2

# See:
# https://docs.aws.amazon.com/enclaves/latest/user/nitro-enclave-cli-install.html#install-cli
RUN amazon-linux-extras install aws-nitro-enclaves-cli
RUN yum install aws-nitro-enclaves-cli-devel -y

# Now turn the local Docker image into an Enclave Image File (EIF).
CMD ["/bin/bash", "-c", \
     "nitro-cli build-enclave --docker-uri $ia2_image --output-file dummy.eif 2>/dev/null"]
EOF

verify_image=$(docker build --quiet . | cut -d ':' -f 2)
pcr0=$(docker run -ti -v /var/run/docker.sock:/var/run/docker.sock "$verify_image" | \
       jq --raw-output ".Measurements.PCR0")
echo "Local enclave image PCR0: $pcr0"

# Determine a random nonce.
nonce=$(head -c 20 /dev/urandom | base64)

curl --insecure -i -X POST https://nitro.nymity.ch:8080/attest -d "nonce=$nonce"
