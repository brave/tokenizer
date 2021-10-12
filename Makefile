.PHONY: all test lint docker ia2 clean

all: test lint ia2

test:
	go test -cover ./...

lint:
	golangci-lint run -E gofmt -E golint --exclude-use-default=false

docker:
	docker build -t ia2 .

ia2:
	go build -o ia2 .

clean:
	rm ia2
