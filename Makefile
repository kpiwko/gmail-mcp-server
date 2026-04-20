.PHONY: all build test test-short test-coverage lint fmt tidy clean install release-snapshot release help

# Build variables
BINARY_NAME := gmail-mcp-server
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS     := -ldflags "-X main.Version=$(VERSION) -s -w"

# Go commands
GOCMD  := go
GOBUILD := $(GOCMD) build
GOTEST  := $(GOCMD) test
GOMOD   := $(GOCMD) mod
GOLINT  := golangci-lint

# Directories
BUILD_DIR := bin

# Default target
all: lint test build

# Build the binary
build:
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/gmail-mcp-server

# Run tests with race detector and coverage
test:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

# Run tests, skipping slow/integration tests
test-short:
	$(GOTEST) -v -short ./...

# Open an HTML coverage report (run 'make test' first)
test-coverage: test
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report written to coverage.html"

# Run linter
lint:
	$(GOLINT) run ./...

# Format code
fmt:
	$(GOCMD) fmt ./...
	goimports -w .

# Tidy and verify dependencies
tidy:
	$(GOMOD) tidy
	$(GOMOD) verify

# Install binary to $GOPATH/bin
install:
	$(GOBUILD) $(LDFLAGS) -o $(GOPATH)/bin/$(BINARY_NAME) ./cmd/gmail-mcp-server

# Build a local snapshot with GoReleaser (no publish)
release-snapshot:
	goreleaser release --snapshot --clean

# Publish a tagged release (requires GITHUB_TOKEN)
release:
	goreleaser release --clean

# Remove build artefacts
clean:
	rm -rf $(BUILD_DIR) dist/
	rm -f coverage.out coverage.html

# Print available targets
help:
	@grep -E '^[a-zA-Z_-]+:' $(MAKEFILE_LIST) | grep -v '^\.PHONY' | \
		awk -F: '{printf "  \033[36m%-20s\033[0m\n", $$1}'
