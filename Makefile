BINARY := opendraft
CMD := ./cmd/opendraft

.PHONY: build run test vet fmt check clean

build:
	go build -o bin/$(BINARY) $(CMD)

run:
	go run $(CMD)

test:
	go test -v -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

check: vet test

clean:
	rm -rf bin/ coverage.txt
