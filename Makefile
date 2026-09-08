BINARY ?= bin/wabi
VERSION ?= dev
LDFLAGS := -X main.version=$(VERSION)

.PHONY: all build test vet fmt check clean demo-build scratch-build demo release-snapshot

all: check build

build:
	@mkdir -p bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/wabi

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w ./cmd ./internal

check:
	@test -z "$$(gofmt -l ./cmd ./internal)" || (echo "gofmt required:"; gofmt -l ./cmd ./internal; exit 1)
	go vet ./...
	go test ./...

clean:
	rm -rf bin out dist

demo-build:
	docker build -t wabi-demo/payment-api:v1 examples/payment-api/v1
	docker build -t wabi-demo/payment-api:v2 examples/payment-api/v2

scratch-build:
	docker build -t wabi-demo/scratch-app:v1 examples/scratch-app

demo: build demo-build
	@mkdir -p out
	$(BINARY) compare wabi-demo/payment-api:v1 wabi-demo/payment-api:v2 \
		--scenario examples/payment-api/scenario.json \
		--target examples/payment-api/compose.yaml \
		--service payment-api \
		--graphs-dir out/graphs \
		--json out/result.json \
		--fail-on never

release-snapshot:
	@mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/wabi-linux-amd64 ./cmd/wabi
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/wabi-linux-arm64 ./cmd/wabi
	GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/wabi-darwin-amd64 ./cmd/wabi
	GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/wabi-darwin-arm64 ./cmd/wabi
