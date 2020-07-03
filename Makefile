.PHONY: clean check test build run-service-mock

TAG_NAME := $(shell git tag -l --contains HEAD)
SHA := $(shell git rev-parse --short HEAD)
VERSION := $(if $(TAG_NAME),$(TAG_NAME),$(SHA))
BUILD_DATE := $(shell date -u '+%Y-%m-%d_%I:%M:%S%p')

default: clean check test build

clean:
	rm -rf cover.out

test: clean
	go test -v -cover ./...

build: clean
	@echo Version: $(VERSION) $(BUILD_DATE)
	CGO_ENABLED=0 go build -v -ldflags '-X "main.version=${VERSION}" -X "main.commit=${SHA}" -X "main.date=${BUILD_DATE}"'

image:
	docker build -t containous/piceus:$(VERSION) .

publish:
	docker push containous/piceus:$(VERSION)

publish-latest:
	docker tag containous/piceus:$(VERSION) containous/piceus:latest
	docker push containous/piceus:latest

check:
	golangci-lint run

run-service-mock:
	go run ./internal/stub/