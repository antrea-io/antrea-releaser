.PHONY: all bin clean generate check golangci golangci-fix help

GOLANGCI_LINT_VERSION := v2.5.0
GOLANGCI_LINT_BINDIR  := .golangci-bin
GOLANGCI_LINT_BIN     := $(GOLANGCI_LINT_BINDIR)/$(GOLANGCI_LINT_VERSION)/golangci-lint

# Default target
all: bin

# Build the prepare-changelog binary
bin:
	@echo "Building prepare-changelog..."
	@mkdir -p bin
	@go build -o bin/prepare-changelog ./cmd/prepare-changelog
	@echo "Binary created: bin/prepare-changelog"

# Generate mocks for testing
generate:
	@echo "Generating mocks..."
	@go run go.uber.org/mock/mockgen@latest -destination=pkg/changelog/mocks/mock_model_caller.go -package=mocks github.com/antrea-io/antrea-releaser/pkg/changelog/types ModelCaller
	@go run go.uber.org/mock/mockgen@latest -destination=pkg/changelog/mocks/mock_github_client.go -package=mocks github.com/antrea-io/antrea-releaser/pkg/changelog/types GitHubClient
	@echo "Mock generation complete"

# Run tests
check:
	@echo "Running tests..."
	@go test -v -race ./...
	@echo "Tests complete"

$(GOLANGCI_LINT_BIN):
	@echo "===> Installing golangci-lint <==="
	@rm -rf $(GOLANGCI_LINT_BINDIR)/* # remove old versions
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOLANGCI_LINT_BINDIR)/$(GOLANGCI_LINT_VERSION) $(GOLANGCI_LINT_VERSION)

# Run golangci-lint
golangci: $(GOLANGCI_LINT_BIN)
	@echo "===> Running golangci-lint <==="
	@$(GOLANGCI_LINT_BIN) run -c .golangci.yml

# Run golangci-lint with --fix
golangci-fix: $(GOLANGCI_LINT_BIN)
	@echo "===> Running golangci-lint --fix <==="
	@$(GOLANGCI_LINT_BIN) run -c .golangci.yml --fix

# Clean build artifacts and generated files
clean:
	@echo "Cleaning build artifacts and generated files..."
	@rm -rf bin
	@rm -rf $(GOLANGCI_LINT_BINDIR)
	@rm -f changelog-model-prompt-*.txt
	@rm -f changelog-model-output-*.json
	@rm -f changelog-model-details-*.json
	@echo "Clean complete"

# Display help
help:
	@echo "Available targets:"
	@echo "  make bin          - Build the prepare-changelog binary in bin/"
	@echo "  make generate     - Generate mocks for testing"
	@echo "  make check        - Run tests"
	@echo "  make golangci     - Run golangci-lint"
	@echo "  make golangci-fix - Run golangci-lint with --fix"
	@echo "  make clean        - Remove build artifacts and generated model files"
	@echo "  make all          - Build everything (same as 'make bin')"
	@echo "  make help         - Show this help message"

