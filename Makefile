.PHONY: all build test lint fmt check clean install

# Default target
all: check build

# Build the binary
build:
	go build -o rvn ./cmd/rvn

# Run tests
test:
	go test -race ./...

# Run tests with coverage
test-coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run linter (requires: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
lint:
	golangci-lint run

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
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest

# Tidy go.mod
tidy:
	go mod tidy

# Quick check - just fmt and vet (faster than full lint)
quick:
	go fmt ./...
	go vet ./...
