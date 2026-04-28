SHELL := /bin/bash
.SILENT:

BINARY  := gpipe
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X github.com/thomaslaurenson/gpipe/cmd.Version=$(VERSION)

.PHONY: help \
	build install \
	fmt fmt_check mod_check vet \
	test test_verbose test_coverage \
	ci \
	clean

help: ## Show this help message
	grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

# BUILD
build: ## Build the binary
	go build -ldflags="$(LDFLAGS)" -o bin/$(BINARY) .

install: ## Install to GOPATH/bin
	go install -ldflags="$(LDFLAGS)" .

# LINT
fmt: ## Format all Go source files
	gofmt -w .

fmt_check: ## Check formatting without writing
	unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		printf 'Unformatted files:\n%s\n' "$$unformatted"; \
		exit 1; \
	fi

mod_check: ## Check go.mod and go.sum are tidy
	go mod tidy
	git diff --exit-code go.mod go.sum

vet: ## Run go vet
	go vet ./...

# TEST
test: ## Run all tests
	go test -race -count=1 ./...

test_verbose: ## Run all tests with verbose output
	go test -race -count=1 -v ./...

test_coverage: ## Run tests and print coverage
	go test -race -count=1 -coverpkg=./internal/... -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	rm coverage.out

ci: fmt_check mod_check vet test ## Run all CI checks locally

# TASKS
clean: ## Remove build artifacts
	rm -rf bin/ dist/ install.sh install.ps1 checksums.txt
