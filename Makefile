# Go parameters
GOCMD = go
BUILD_DIR = build
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOTEST = $(GOCMD) test

# Build target
BINARY_NAME = postmanpat
BUILT_BINARY = $(BUILD_DIR)/$(BINARY_NAME)

all: test build

build:
	$(GOBUILD) -o $(BUILT_BINARY) -v ./cmd/postmanpat

test:
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -f $(BUILT_BINARY)

run: build
	./$(BUILT_BINARY)

.PHONY: all build test clean run