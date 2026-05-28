SHELL := /bin/bash

BINARY  := gpipe
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X github.com/thomaslaurenson/gpipe/cmd.Version=$(VERSION)

TAG ?= $(shell git describe --tags --abbrev=0 2>/dev/null)

.PHONY: help
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  %-24s %s\n", $$1, $$2}'

# BUILD
.PHONY: build
build: ## Build the binary
	go build -ldflags="$(LDFLAGS)" -o bin/$(BINARY) .

.PHONY: install
install: ## Install to GOPATH/bin
	go install -ldflags="$(LDFLAGS)" .

# LINT
.PHONY: fmt
fmt: ## Format all Go source files
	gofmt -w .

.PHONY: fmt_check
fmt_check: ## Check formatting without writing
	unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		printf 'Unformatted files:\n%s\n' "$$unformatted"; \
		exit 1; \
	fi

.PHONY: mod_check
mod_check: ## Check go.mod and go.sum are tidy
	go mod tidy
	git diff --exit-code go.mod go.sum

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint_bash
lint_bash: generate_test_fixtures ## Run shellcheck on the rendered install.sh fixture
	@printf 'bash -n  test/fixtures/install_rendered.sh ... '
	@bash -n test/fixtures/install_rendered.sh \
	  && printf 'ok\n' \
	  || { printf 'fail\n'; exit 1; }
	@printf 'shellcheck  test/fixtures/install_rendered.sh ... '
	@shellcheck test/fixtures/install_rendered.sh \
	  && printf 'ok\n' \
	  || { printf 'fail\n'; exit 1; }

.PHONY: lint_ps
lint_ps: generate_test_fixtures ## Run PSScriptAnalyzer on the rendered install.ps1 fixture (requires pwsh)
	@printf 'PSScriptAnalyzer  test/fixtures/install_rendered.ps1 ... '
	@pwsh -NoProfile -NonInteractive -Command \
	  "Import-Module PSScriptAnalyzer; \$$r = Invoke-ScriptAnalyzer -Path 'test/fixtures/install_rendered.ps1' -Severity Warning,Error -ExcludeRule 'PSAvoidUsingWriteHost','PSUseShouldProcessForStateChangingFunctions'; if (\$$r) { \$$r | Format-Table -AutoSize; exit 1 }" \
	  && printf 'ok\n' \
	  || { printf 'fail\n'; exit 1; }

# TEST
.PHONY: generate_test_fixtures
generate_test_fixtures: ## Regenerate test/fixtures from current templates
	go run ./test/cmd/generate_fixtures

.PHONY: test
test: ## Run all tests (Go + bats + Pester)
	go test -race -count=1 ./...
	$(MAKE) test_bash
	$(MAKE) test_ps

.PHONY: test_verbose
test_verbose: ## Run all tests with verbose output
	go test -race -count=1 -v ./...
	$(MAKE) test_bash_verbose
	$(MAKE) test_ps_verbose

.PHONY: test_coverage
test_coverage: ## Run tests and print coverage
	go test -race -count=1 -coverpkg=./internal/... -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	rm coverage.out

.PHONY: test_bash
test_bash: generate_test_fixtures ## Run bats tests for install.sh template
	test/extern/bats/bin/bats test/install_sh.bats

.PHONY: test_bash_verbose
test_bash_verbose: generate_test_fixtures ## Run bats tests with verbose output
	test/extern/bats/bin/bats --verbose-run test/install_sh.bats

.PHONY: test_ps
test_ps: generate_test_fixtures ## Run Pester tests for install.ps1 template (requires pwsh + Pester >= 5.0)
	pwsh -NoProfile -NonInteractive -Command \
	  "Import-Module Pester -MinimumVersion 5.0; \$$cfg = New-PesterConfiguration; \$$cfg.Run.Path = 'test/install_ps1.Tests.ps1'; \$$cfg.Output.Verbosity = 'Detailed'; \$$cfg.Run.Exit = \$$true; Invoke-Pester -Configuration \$$cfg"

.PHONY: test_ps_verbose
test_ps_verbose: generate_test_fixtures ## Run Pester tests with diagnostic output
	pwsh -NoProfile -NonInteractive -Command \
	  "Import-Module Pester -MinimumVersion 5.0; \$$cfg = New-PesterConfiguration; \$$cfg.Run.Path = 'test/install_ps1.Tests.ps1'; \$$cfg.Output.Verbosity = 'Diagnostic'; \$$cfg.Run.Exit = \$$true; Invoke-Pester -Configuration \$$cfg"

.PHONY: ci
ci: fmt_check mod_check vet lint_bash lint_ps test ## Run all CI checks locally

# GET
.PHONY: get_changelog
get_changelog: ## Print release notes for TAG to stdout (default: latest tag; override with TAG=v1.0.0)
	@tag="$(TAG)"; tag="$${tag#v}"; \
	if [[ -z "$$tag" ]]; then \
	  printf 'get_changelog: TAG is empty; pass TAG=v1.0.0 or create a git tag\n' >&2; \
	  exit 1; \
	fi; \
	notes="$$(awk -v tag="$$tag" ' \
	  /^## / { if (found) exit; if (index($$0,"## "tag" ")==1 || $$0=="## "tag) found=1; next } \
	  found { lines[n++]=$$0 } \
	  END { \
	    s=0; while (s<n && lines[s]~/^[[:space:]]*$$/) s++; \
	    e=n-1; while (e>=s && lines[e]~/^[[:space:]]*$$/) e--; \
	    for (i=s;i<=e;i++) print lines[i] \
	  }' CHANGELOG.md)"; \
	if [[ -z "$$notes" ]]; then \
	  printf 'get_changelog: no CHANGELOG entry for %s\n' "$$tag" >&2; \
	  exit 1; \
	fi; \
	printf '%s\n' "$$notes"

# RELEASE
.PHONY: snapshot
snapshot: ## Build a local snapshot release with goreleaser
	goreleaser build --snapshot --clean

.PHONY: release_check
release_check: ## Validate goreleaser config
	goreleaser check

# TASKS
.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin/ dist/ install.sh install.ps1 checksums.txt
