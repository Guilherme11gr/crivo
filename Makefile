.PHONY: build test clean install lint build-all

# Single source of truth for local builds: `make build` produces a binary that
# reports "dev" unless VERSION is set. Release builds (GoReleaser) inject the
# version via ldflags independently — see .goreleaser.yaml.
VERSION ?= dev
BINARY = crivo
LDFLAGS = -s -w -X main.version=$(VERSION)

# The binary is named `crivo` on every OS (no .exe suffix); Windows users get
# crivo.exe from the release archives instead. -buildvcs=false keeps builds
# reproducible (and avoids VCS stamping failures in git worktrees).
build:
	go build -buildvcs=false -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/crivo/

test:
	go test ./...

clean:
	rm -f $(BINARY)

install:
	go install ./cmd/crivo/

lint:
	go vet ./...

# Cross-compile
build-all:
	GOOS=linux GOARCH=amd64 go build -buildvcs=false -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64 ./cmd/crivo/
	GOOS=darwin GOARCH=amd64 go build -buildvcs=false -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-amd64 ./cmd/crivo/
	GOOS=darwin GOARCH=arm64 go build -buildvcs=false -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-arm64 ./cmd/crivo/
	GOOS=windows GOARCH=amd64 go build -buildvcs=false -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-windows-amd64.exe ./cmd/crivo/
