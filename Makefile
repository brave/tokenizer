binary = tkzr
tmp_image = $(binary)-repro.tar
godeps = *.go go.mod go.sum
browser = ${BROWSER}
cover_out = cover.out
cover_html = cover.html

.PHONY: all
all: test lint $(binary)

.PHONY: test
test:
	go test -cover ./...

.PHONY: lint
lint:
	golangci-lint run

.PHONY: coverage
coverage:
	go test -coverprofile=$(cover_out) .
	go tool cover -html=$(cover_out) -o $(cover_html)
	${BROWSER} $(shell realpath cover.html)
	@sleep 1 # Give the browser a second to open the file.
	rm -f $(cover_out) $(cover_html)

.PHONY: image
image:
	$(eval IMAGE=$(shell ko publish --local . 2>/dev/null))
	@echo "Built image URI: $(IMAGE)."
	$(eval DIGEST=$(shell echo $(IMAGE) | cut -d ':' -f 2))
	@echo "SHA-256 digest: $(DIGEST)"

.PHONY: eif
eif: image
	nitro-cli build-enclave --docker-uri $(IMAGE) --output-file ko.eif
	$(eval ENCLAVE_ID=$(shell nitro-cli describe-enclaves | jq -r '.[0].EnclaveID'))
	@if [ "$(ENCLAVE_ID)" != "null" ]; then nitro-cli terminate-enclave --enclave-id $(ENCLAVE_ID); fi
	@echo "Starting enclave."
	nitro-cli run-enclave --cpu-count 2 --memory 2500 --enclave-cid 4 --eif-path ko.eif --debug-mode
	@echo "Showing enclave logs."
	nitro-cli console --enclave-id $$(nitro-cli describe-enclaves | jq -r '.[0].EnclaveID')

.PHONY: docker
docker:
	docker run \
		-v $(PWD):/workspace \
		--network=host \
		gcr.io/kaniko-project/executor:v1.7.0 \
		--reproducible \
		--dockerfile /workspace/Dockerfile \
		--no-push \
		--tarPath /workspace/$(tmp_image) \
		--destination tkzr \
		--context dir:///workspace/ && cat $(tmp_image) | docker load
	rm -f $(tmp_image)

.PHONY: update-deps
update-deps:
	go get -u ./...
	go mod tidy
	go mod vendor

$(binary): $(godeps)
	go build -o $(binary)

.PHONY: clean
clean:
	rm -f $(binary) $(tmp_image)
