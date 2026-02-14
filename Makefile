.PHONY: all build test test-integration test-all lint fmt check clean install

GOLANGCI_LINT_VERSION ?= v2.9.0
GOLANGCI_LINT_MODULE := github.com/golangci/golangci-lint/v2/cmd/golangci-lint

# Default target
all: check build

# Build the binary
build:
	go build -o rvn ./cmd/rvn

# Run unit tests (fast, no build required)
test:
	go test -race ./...

# Run integration tests (slower, builds CLI binary)
test-integration:
	go test -race -tags=integration -v ./internal/cli ./internal/mcp

# Run all tests (unit + integration)
test-all: test test-integration

# Run tests with coverage
test-coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run linter with golangci-lint v2.
# Uses local v2 binary when available, otherwise runs pinned version via go run.
lint:
	@version="$$(golangci-lint --version 2>/dev/null || true)"; \
	case "$$version" in \
		*"version v2."*|*"version 2."*) golangci-lint run ;; \
		*) \
			echo "golangci-lint v2 not found in PATH; running $(GOLANGCI_LINT_MODULE)@$(GOLANGCI_LINT_VERSION)"; \
			go run $(GOLANGCI_LINT_MODULE)@$(GOLANGCI_LINT_VERSION) run; \
			;; \
	esac

# Format code
fmt:
	gofmt -s -w .
	goimports -w -local github.com/aidanlsb/raven .

# Check formatting without modifying files
fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Run 'make fmt' to fix formatting" && gofmt -l . && exit 1)

# Run all checks (formatting, linting, tests)
check: fmt-check lint test

# Clean build artifacts
clean:
	rm -f rvn coverage.out coverage.html

# Install the binary to $GOPATH/bin
install:
	go install ./cmd/rvn

# Install development tools
tools:
	go install $(GOLANGCI_LINT_MODULE)@$(GOLANGCI_LINT_VERSION)
	go install golang.org/x/tools/cmd/goimports@latest

# Tidy go.mod
tidy:
	go mod tidy

# Quick check - just fmt and vet (faster than full lint)
quick:
	go fmt ./...
	go vet ./...
