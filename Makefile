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

all: test build

# TODO: better build names seperating the Go build from the web service
ws-build:
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

lint:
	golangci-lint run --config .golangci.yml

.PHONY: all build test clean run