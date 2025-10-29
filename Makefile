.PHONY: build clean help

BINARY_NAME=gpustat-exporter

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) .
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
