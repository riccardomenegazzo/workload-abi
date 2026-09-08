BINARY := bin/wabi

.PHONY: build test vet fmt clean

build:
	go build -trimpath -o $(BINARY) ./cmd/wabi

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w ./cmd ./internal

clean:
	rm -rf bin
