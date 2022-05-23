.PHONY: clean check test build run-service-mock

TAG_NAME := $(shell git tag -l --contains HEAD)
SHA := $(shell git rev-parse --short HEAD)
VERSION := $(if $(TAG_NAME),$(TAG_NAME),$(SHA))
BUILD_DATE := $(shell date -u '+%Y-%m-%d_%I:%M:%S%p')
BIN_NAME := "piceus"

# Default build target
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
DOCKER_BUILD_PLATFORMS ?= linux/amd64,linux/arm64
default: clean check test build

clean:
	rm -rf cover.out

test: clean
	go test -v -cover ./...

build: clean
	@echo Version: $(VERSION) $(BUILD_DATE)
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build -v -ldflags '-X "main.version=${VERSION}" -X "main.commit=${SHA}" -X "main.date=${BUILD_DATE}"' -o "./dist/${GOOS}/${GOARCH}/${BIN_NAME}"

build-linux-arm64: export GOOS := linux
build-linux-arm64: export GOARCH := arm64
build-linux-arm64:
	make build

build-linux-amd64: export GOOS := linux
build-linux-amd64: export GOARCH := amd64
build-linux-amd64:
	make build

## Build Multi archs Docker image
multi-arch-image-%: build-linux-amd64 build-linux-arm64
	docker buildx build $(DOCKER_BUILDX_ARGS) --progress=chain -t gcr.io/traefiklabs/$(BIN_NAME):$* --platform=$(DOCKER_BUILD_PLATFORMS) -f buildx.Dockerfile .

image:
	docker build -t gcr.io/traefiklabs/piceus:$(VERSION) .

publish:
	docker push gcr.io/traefiklabs/piceus:$(VERSION)

publish-latest:
	docker tag gcr.io/traefiklabs/piceus:$(VERSION) gcr.io/traefiklabs/piceus:latest
	docker push gcr.io/traefiklabs/piceus:latest

check:
	golangci-lint run

run-service-mock:
	go run ./internal/stub/
