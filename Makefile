.PHONY: build test clean install

VERSION ?= 0.1.0
BINARY = crivo
LDFLAGS = -s -w

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY).exe ./cmd/crivo/

test:
	go test ./...

clean:
	rm -f $(BINARY).exe

install:
	go install ./cmd/crivo/

lint:
	go vet ./...

# Cross-compile
build-all:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64 ./cmd/crivo/
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-amd64 ./cmd/crivo/
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-arm64 ./cmd/crivo/
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-windows-amd64.exe ./cmd/crivo/
