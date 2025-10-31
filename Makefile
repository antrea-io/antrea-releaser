.PHONY: all bin clean help

# Default target
all: bin

# Build the prepare-changelog binary
bin:
	@echo "Building prepare-changelog..."
	@mkdir -p bin
	@go build -o bin/prepare-changelog ./cmd/prepare-changelog
	@echo "Binary created: bin/prepare-changelog"

# Clean build artifacts and generated files
clean:
	@echo "Cleaning build artifacts and generated files..."
	@rm -rf bin
	@rm -f changelog-model-prompt-*.txt
	@rm -f changelog-model-output-*.json
	@rm -f changelog-model-details-*.json
	@echo "Clean complete"

# Display help
help:
	@echo "Available targets:"
	@echo "  make bin     - Build the prepare-changelog binary in bin/"
	@echo "  make clean   - Remove build artifacts and generated model files"
	@echo "  make all     - Build everything (same as 'make bin')"
	@echo "  make help    - Show this help message"

