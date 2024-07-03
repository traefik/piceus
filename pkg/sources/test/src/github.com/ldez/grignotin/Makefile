.PHONY: check test

export GO111MODULE=on

default: check test

test:
	go test -v -cover ./...

check:
	golangci-lint run
