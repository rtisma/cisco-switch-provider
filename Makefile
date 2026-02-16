.PHONY: build install test test-unit test-integration test-all test-coverage test-safety clean fmt lint

# Variables
BINARY_NAME=terraform-provider-cisco
VERSION?=1.0.0
OS_ARCH?=$(shell go env GOOS)_$(shell go env GOARCH)
INSTALL_PATH=~/.terraform.d/plugins/registry.terraform.io/example-org/cisco/$(VERSION)/$(OS_ARCH)

# Build the provider
build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) -ldflags="-X main.version=$(VERSION)"

# Install the provider locally for testing
install: build
	@echo "Installing $(BINARY_NAME) to $(INSTALL_PATH)..."
	@mkdir -p $(INSTALL_PATH)
	@cp $(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME)
	@chmod +x $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "Provider installed successfully!"
	@echo "You can now use it in your Terraform configurations."

# Run all tests
test: test-unit test-integration
	@echo "All tests completed!"

# Run unit tests only (parser, validation, etc.)
test-unit:
	@echo "Running unit tests..."
	go test -v ./internal/provider/client -run "TestParse|TestIsError|TestVlan.*Validation"
	go test -v ./internal/provider/resources -run "TestVlan.*Validation|TestBuild"

# Run integration tests (requires mock switch)
test-integration:
	@echo "Running integration tests with mock switch..."
	go test -v ./internal/provider/client -run "TestClient|TestVLAN|TestInterface|TestSVI|TestConcurrent|TestInvalid"

# Run comprehensive test suite with safety checks
test-all:
	@echo "Running comprehensive test suite..."
	@./test.sh

# Run safety tests before production use
test-safety:
	@echo "Running safety validation tests..."
	@echo "⚠️  These tests ensure the provider won't damage your network"
	@./test.sh
	@echo ""
	@echo "✅ Safety tests passed!"
	@echo ""
	@echo "📋 Before using on real hardware, please read:"
	@echo "   - SAFETY.md for safety guidelines"
	@echo "   - Test on non-production hardware first"
	@echo "   - Have a rollback plan ready"

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Run linter
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install it from https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -f $(BINARY_NAME)
	@rm -f coverage.out coverage.html
	@rm -rf dist/
	@echo "Clean complete!"

# Generate documentation
docs:
	@echo "Generating documentation..."
	@which tfplugindocs > /dev/null || go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest
	tfplugindocs generate

# Run the provider in debug mode
debug: build
	@echo "Running provider in debug mode..."
	./$(BINARY_NAME) -debug

# Update dependencies
deps:
	@echo "Updating dependencies..."
	go mod tidy
	go mod verify

# Show help
help:
	@echo "Available targets:"
	@echo "  build           - Build the provider binary"
	@echo "  install         - Build and install the provider locally"
	@echo "  test            - Run all tests (unit + integration)"
	@echo "  test-unit       - Run unit tests only"
	@echo "  test-integration- Run integration tests with mock switch"
	@echo "  test-all        - Run comprehensive test suite"
	@echo "  test-safety     - Run safety validation (before production use)"
	@echo "  test-coverage   - Run tests with coverage report"
	@echo "  fmt             - Format code"
	@echo "  lint            - Run linter"
	@echo "  clean           - Clean build artifacts"
	@echo "  docs            - Generate documentation"
	@echo "  debug           - Run provider in debug mode"
	@echo "  deps            - Update dependencies"
	@echo "  help            - Show this help message"
