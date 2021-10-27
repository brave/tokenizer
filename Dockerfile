FROM public.ecr.aws/amazonlinux/amazonlinux:2

# See:
# https://docs.aws.amazon.com/enclaves/latest/user/nitro-enclave-cli-install.html#install-cli
RUN amazon-linux-extras install aws-nitro-enclaves-cli
RUN yum install aws-nitro-enclaves-cli-devel -y

# Now turn the local Docker image into an Enclave Image File (EIF).
CMD ["/bin/bash", "-c",      "nitro-cli build-enclave --docker-uri ko.local/v2-82ee1e512d39f6027f9214614ec7f798:9600395768007f53a73bee75a9b1a9a3dfba86829c15e32791bbe3814b25dfcf --output-file dummy.eif 2>/dev/null"]
