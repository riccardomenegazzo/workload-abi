BINARY := bin/wabi
NATIVE_BINARY := bin/wabi-native
IMAGE ?= workload-abi:dev
VERSION ?= dev
HOST_GOOS := $(shell go env GOOS)

.PHONY: build build-native test vet fmt fmt-check schemas-check check docker-build clean

build:
	go build -trimpath -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/wabi
	@if [ "$(HOST_GOOS)" = "linux" ]; then \
		go build -trimpath -ldflags "-X main.version=$(VERSION)" -o $(NATIVE_BINARY) ./cmd/wabi-native; \
	fi

build-native:
	@if [ "$(HOST_GOOS)" != "linux" ]; then \
		echo "wabi-native is supported only on Linux" >&2; \
		exit 1; \
	fi
	go build -trimpath -ldflags "-X main.version=$(VERSION)" -o $(NATIVE_BINARY) ./cmd/wabi-native

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w ./cmd ./internal

fmt-check:
	@test -z "$$(gofmt -l ./cmd ./internal)" || (gofmt -l ./cmd ./internal && exit 1)

schemas-check:
	@python3 -c 'import json,pathlib; files=list(pathlib.Path("schemas").rglob("*.json")); assert files, "no schemas"; [json.load(p.open()) for p in files]; print("validated", len(files), "schemas")'

check: fmt-check schemas-check vet test build

docker-build:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE) .

clean:
	rm -rf bin dist
