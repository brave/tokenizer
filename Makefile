.PHONY: all test lint eif ia2 clean

all: test lint ia2

test:
	go test -cover ./...

lint:
	golangci-lint run -E gofmt -E golint --exclude-use-default=false

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

ia2:
	go build -o ia2 .

clean:
	rm ia2
