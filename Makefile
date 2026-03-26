# Variables
BIN := $(CURDIR)/bin
STATICCHECK := $(BIN)/staticcheck
GO_FILES := $(shell find . -type f -name "*.go" -not -path "./vendor/*" -not -path "./.git/*")

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
NC := \033[0m # No Color

.PHONY: all
all: clean lint test

# Create bin directory
$(BIN):
	@mkdir -p $(BIN)

# Install staticcheck from go.mod (respects version pin)
$(STATICCHECK): go.mod go.sum | $(BIN)
	@echo "Installing staticcheck from go.mod..."
	@go build -o $(STATICCHECK) honnef.co/go/tools/cmd/staticcheck

.PHONY: lint
lint: $(STATICCHECK)
	@echo "$(GREEN)Running staticcheck...$(NC)"
	@$(STATICCHECK) ./...
	@echo "$(GREEN)✓ Staticcheck passed$(NC)"

.PHONY: lint-verbose
lint-verbose: $(STATICCHECK)
	$(STATICCHECK) -f stylish ./...

.PHONY: format
format:
	@echo "$(GREEN)Running gofmt...$(NC)"
	@gofmt -w $(GO_FILES)
	@echo "$(GREEN)✓ gofmt complete$(NC)"

# Check if tools are up to date with go.mod
.PHONY: tools-check
tools-check:
	@if [ ! -f $(STATICCHECK) ] || [ go.mod -nt $(STATICCHECK) ]; then \
		echo "Tools need update..."; \
		$(MAKE) $(STATICCHECK); \
	fi

.PHONY: clean
clean:
	rm -rf $(BIN)

.PHONY: update-tools
update-tools:
	go get -tool honnef.co/go/tools/cmd/staticcheck@latest
	go mod tidy
	rm -f $(STATICCHECK)  # Force rebuild

.PHONY: test
test: $(STATICCHECK)
	@echo "$(GREEN)Running test...$(NC)"
	@go test ./...
	@echo "$(GREEN)✓ Go Test passed$(NC)"
