export PATH := /usr/local/go/bin:$(HOME)/go/bin:$(PATH)
GO := /usr/local/go/bin/go
GOLANGCI_LINT := $(HOME)/go/bin/golangci-lint
GO_MODULE := github.com/persistorai/persistor
GO_PACKAGES := ./...
GO_TEST_FLAGS := -v -race
BINARY_DIR := bin

VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || cat VERSION)
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.buildDate=$(BUILD_DATE)

.PHONY: build build-server build-cli clean test test-race test-coverage lint lint-fix lint-md format vet ci run deps tidy setup-hooks install install-server install-cli

## Build both binaries.
build: build-server build-cli

## Build the server binary.
build-server:
	@echo "Building persistor-server..."
	@mkdir -p $(BINARY_DIR)
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BINARY_DIR)/persistor-server ./cmd/server

## Build the CLI binary.
build-cli:
	@echo "Building persistor..."
	@mkdir -p $(BINARY_DIR)
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BINARY_DIR)/persistor ./cmd/persistor-cli

## Clean build artifacts.
clean:
	@echo "Cleaning artifacts..."
	@rm -rf $(BINARY_DIR)
	@$(GO) clean -cache -testcache

## Run tests.
test:
	@echo "Running tests..."
	$(GO) test $(GO_TEST_FLAGS) $(GO_PACKAGES)

## Run tests with race detection.
test-race:
	@echo "Running tests with race detection..."
	$(GO) test -race -v $(GO_PACKAGES)

## Run tests with coverage.
test-coverage:
	@echo "Running tests with coverage..."
	@mkdir -p coverage
	$(GO) test -race -coverprofile=coverage/coverage.out -covermode=atomic $(GO_PACKAGES)
	$(GO) tool cover -html=coverage/coverage.out -o coverage/coverage.html
	@echo "Coverage report: coverage/coverage.html"

## Run golangci-lint.
lint:
	@echo "Linting..."
	$(GOLANGCI_LINT) run $(GO_PACKAGES)

## Run golangci-lint with auto-fix.
lint-fix:
	@echo "Fixing lint issues..."
	$(GOLANGCI_LINT) run --fix $(GO_PACKAGES)

## Format code.
format:
	@echo "Formatting..."
	gofmt -s -w .
	goimports -w -local $(GO_MODULE) .

## Run go vet.
vet:
	@echo "Running go vet..."
	$(GO) vet $(GO_PACKAGES)

## Lint markdown files.
lint-md:
	@echo "Linting markdown..."
	markdownlint '**/*.md' --ignore vendor --ignore node_modules

## Run full CI checks.
ci: format vet lint lint-md test-coverage
	@echo "CI checks passed!"

## Install git hooks.
setup-hooks:
	@echo "Installing git hooks..."
	@cp scripts/hooks/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Pre-commit hook installed."

## Run the server (loads .env if present).
run: build-server
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi; ./$(BINARY_DIR)/persistor-server

## Install both binaries.
install: install-server install-cli

## Install the server binary and restart service.
install-server: build-server
	sudo cp bin/persistor-server /usr/local/bin/persistor-server
	sudo systemctl restart persistor.service
	@echo "Installed persistor-server and restarted persistor.service"

## Install the CLI binary.
install-cli: build-cli
	sudo cp bin/persistor /usr/local/bin/persistor
	@echo "Installed persistor to /usr/local/bin/persistor"

## Download dependencies.
deps:
	@echo "Downloading dependencies..."
	$(GO) mod download
	$(GO) mod verify

## Tidy dependencies.
tidy:
	@echo "Tidying dependencies..."
	$(GO) mod tidy
