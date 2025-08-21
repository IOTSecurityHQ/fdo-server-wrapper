.PHONY: build clean test run help

# Build variables
BINARY_NAME=fdo-proxy
BUILD_DIR=build
MAIN_PATH=./cmd/server

# Default target
all: build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@go clean
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	go test ./...
	@echo "Tests complete"

# Run the proxy with basic configuration
run: build
	@echo "Starting FDO proxy server..."
	./$(BUILD_DIR)/$(BINARY_NAME) -listen localhost:8080 -debug

# Run with ledger integration (example)
run-ledger: build
	@echo "Starting FDO proxy server with ledger integration..."
	./$(BUILD_DIR)/$(BINARY_NAME) \
		-listen localhost:8080 \
		-ledger-url http://localhost:8081 \
		-ledger-api-key test-key \
		-enable-product-passport \
		-owner-id test-owner \
		-debug

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy
	@echo "Dependencies installed"

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	@echo "Code formatting complete"

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run
	@echo "Linting complete"

# Show help
help:
	@echo "Available targets:"
	@echo "  build       - Build the proxy binary"
	@echo "  clean       - Clean build artifacts"
	@echo "  test        - Run tests"
	@echo "  run         - Run proxy with basic config"
	@echo "  run-ledger  - Run proxy with ledger integration"
	@echo "  deps        - Install dependencies"
	@echo "  fmt         - Format code"
	@echo "  lint        - Lint code"
	@echo "  help        - Show this help" 