.PHONY: build clean help

BINARY_NAME=gpustat-exporter
VERSION?=dev

# Build the binary
build:
	@echo "Building $(BINARY_NAME) version $(VERSION)..."
	go build -ldflags="-X 'main.version=$(VERSION)'" -o $(BINARY_NAME) .
	@echo "Build complete: $(BINARY_NAME)"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	go clean
	@echo "Clean complete"

# Show help
help:
	@echo "Available targets:"
	@echo "  make build   - Build the binary"
	@echo "  make clean   - Clean build artifacts"
	@echo "  make help    - Show this help message"
