.PHONY: build clean test run lint tidy vet run-agent run-gateway help install fmt

# Binary info
BINARY_NAME=dogclaw
CMD_PATH=./cmd/dogclaw
BUILD_DIR=./build

# Detect current platform
UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)

ifeq ($(UNAME_S),Darwin)
	PLATFORM := darwin
else ifeq ($(UNAME_S),Linux)
	PLATFORM := linux
endif

ifeq ($(UNAME_M),x86_64)
	ARCH := amd64
else ifeq ($(UNAME_M),arm64)
	ARCH := arm64
endif

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Build flags
BUILD_TIME=$(shell date '+%Y-%m-%d %H:%M:%S')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS=-ldflags "-s -w -X 'dogclaw/pkg/version.BuildTime=$(BUILD_TIME)' -X 'dogclaw/pkg/version.GitCommit=$(GIT_COMMIT)'"

default: build

# Build the binary (current platform)
build:
	@echo "🔨 Building $(BINARY_NAME) for $(PLATFORM)/$(ARCH)..."
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) $(CMD_PATH)

# Build for multiple platforms
build-darwin:
	@echo "🔨 Building for darwin/amd64..."
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_PATH)

build-darwin-arm64:
	@echo "🔨 Building for darwin/arm64 (Apple Silicon)..."
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_PATH)

build-linux:
	@echo "🔨 Building for linux/amd64..."
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_PATH)

build-linux-arm64:
	@echo "🔨 Building for linux/arm64..."
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(CMD_PATH)

build-linux-armv7:
	@echo "🔨 Building for linux/arm/v7 (32-bit ARM)..."
	GOOS=linux GOARCH=arm GOARM=7 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-armv7 $(CMD_PATH)

build-all: build-darwin build-darwin-arm64 build-linux build-linux-arm64 build-linux-armv7

# Run agent mode
run-agent: build
	@echo "🤖 Running in agent mode..."
	./$(BINARY_NAME) agent

# Run gateway mode
run-gateway: build
	@echo "🌐 Running in gateway mode..."
	./$(BINARY_NAME) gateway

# Run with custom args
run: build
	./$(BINARY_NAME) $(ARGS)

# Install globally
install:
	@echo "📦 Installing $(BINARY_NAME)..."
	$(GOBUILD) $(LDFLAGS) -o $(GOPATH)/bin/$(BINARY_NAME) $(CMD_PATH)

# Clean build artifacts
clean:
	@echo "🧹 Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f $(BINARY_NAME)

# Run tests
test:
	@echo "🧪 Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "🧪 Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Format code
fmt:
	@echo "✨ Formatting code..."
	$(GOFMT) ./...

# Tidy dependencies
tidy:
	@echo "📦 Tidying dependencies..."
	$(GOMOD) tidy

# Vendor dependencies
vendor:
	@echo "📦 Vendoring dependencies..."
	$(GOMOD) vendor

# Lint with go vet
vet:
	@echo "🔍 Running go vet..."
	$(GOVET) ./...

# Alias for vet (backward compatibility)
lint: vet

# Run all checks
check: fmt vet test tidy

# Show help
help:
	@echo "🦞 DogClaw Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build             - Build the binary (default)"
	@echo "  build-darwin      - Build for macOS/amd64"
	@echo "  build-darwin-arm64- Build for macOS/arm64 (Apple Silicon)"
	@echo "  build-linux       - Build for Linux/amd64"
	@echo "  build-linux-arm64 - Build for Linux/arm64"
	@echo "  build-linux-armv7 - Build for Linux/arm/v7 (32-bit ARM)"
	@echo "  build-all         - Build for all platforms"
	@echo "  run-agent         - Build and run in agent mode"
	@echo "  run-gateway       - Build and run in gateway mode"
	@echo "  run               - Build and run with ARGS (make run ARGS=agent)"
	@echo "  install           - Install binary to GOPATH/bin"
	@echo "  clean             - Remove build artifacts"
	@echo "  test              - Run tests"
	@echo "  test-coverage     - Run tests with coverage report"
	@echo "  fmt               - Format code"
	@echo "  tidy              - Tidy dependencies"
	@echo "  vendor            - Vendor dependencies"
	@echo "  vet/lint          - Run go vet"
	@echo "  check             - Run fmt, vet, test, and tidy"
	@echo "  help              - Show this help"
