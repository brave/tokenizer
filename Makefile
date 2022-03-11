.PHONY: all test lint eif ia2 clean

binary = ia2
godeps = *.go go.mod go.sum

all: test lint $(binary)

test:
	go test -cover ./...

lint:
	golangci-lint run -E gofmt -E revive --exclude-use-default=false

image:
	$(eval IMAGE=$(shell ko publish --local . 2>/dev/null))
	@echo "Built image URI: $(IMAGE)."
	$(eval DIGEST=$(shell echo $(IMAGE) | cut -d ':' -f 2))
	@echo "SHA-256 digest: $(DIGEST)"

eif: image
	nitro-cli build-enclave --docker-uri $(IMAGE) --output-file ko.eif
	$(eval ENCLAVE_ID=$(shell nitro-cli describe-enclaves | jq -r '.[0].EnclaveID'))
	@if [ "$(ENCLAVE_ID)" != "null" ]; then nitro-cli terminate-enclave --enclave-id $(ENCLAVE_ID); fi
	@echo "Starting enclave."
	nitro-cli run-enclave --cpu-count 2 --memory 2500 --enclave-cid 4 --eif-path ko.eif --debug-mode
	@echo "Showing enclave logs."
	nitro-cli console --enclave-id $$(nitro-cli describe-enclaves | jq -r '.[0].EnclaveID')

docker:
	docker run \
		-v $(PWD):/workspace \
		--network=host \
		gcr.io/kaniko-project/executor:v1.7.0 \
		--reproducible \
		--dockerfile /workspace/Dockerfile \
		--no-push \
		--tarPath /workspace/ia2-repro.tar \
		--destination ia2 \
		--context dir:///workspace/ && cat ia2-repro.tar | docker load

$(binary): $(godeps)
	go build -o $(binary).

clean:
	rm $(binary)
