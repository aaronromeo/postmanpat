# Go parameters
GOCMD = go
NPMCMD = npm
BUILD_DIR = build
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOTEST = $(GOCMD) test
NPMINSTALL = $(NPMCMD) install
NPMBUILD = $(NPMCMD) run build

# Build target
BINARY_NAME = postmanpat
BUILT_BINARY = $(BUILD_DIR)/$(BINARY_NAME)


# Tooling
BIN_DIR := $(shell go env GOBIN)
ifeq ($(BIN_DIR),)
BIN_DIR := $(shell go env GOPATH)/bin
endif

GOLANGCI := $(BIN_DIR)/golangci-lint
GOLANGCI_VERSION := v1.64.8

.PHONY: install-golangci lint

install-golangci:
	@mkdir -p "$(BIN_DIR)"
	@if [ -x "$(GOLANGCI)" ]; then \
		echo "golangci-lint already installed at $(GOLANGCI)"; \
	else \
		echo "Installing golangci-lint $(GOLANGCI_VERSION) to $(BIN_DIR)"; \
		GO111MODULE=on go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_VERSION); \
		if [ ! -x "$(GOLANGCI)" ]; then \
			echo "Note: $(BIN_DIR) may not be in your PATH. Add it to PATH to use golangci-lint directly."; \
		fi; \
	fi

all: test build

# TODO: better build names seperating the Go build from the web service
build-npm:
	$(NPMINSTALL)
	$(NPMBUILD)

build:
	$(GOBUILD) -o $(BUILT_BINARY) -v ./cmd/postmanpat

test:
	$(GOTEST) -v ./... -coverprofile=./cover.out -covermode=atomic -coverpkg=./...

clean:
	$(GOCLEAN)
	rm -f $(BUILT_BINARY)

# eg. make run ARGS="mn"
run: build
	./$(BUILT_BINARY) $(ARGS)

webserver: build build-npm
	./$(BUILT_BINARY) ws

lint: install-golangci
	$(GOLANGCI) run --config .golangci.yml

.PHONY: all build test clean run