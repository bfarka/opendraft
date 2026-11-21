BINARY := opendraft
CMD := ./cmd/opendraft

.PHONY: build run test vet fmt check hooks clean

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

hooks:
	git config core.hooksPath .githooks

clean:
	rm -rf bin/ coverage.txt
