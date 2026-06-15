# Makefile for the ergo.services/ergo core module.
# Runs the full test suite and a static audit (vet, gofmt, build) over every package.

GO ?= go
COVER ?= coverage.out

.PHONY: help all audit test test-race cover cover-html vet fmt fmt-fix build tidy clean

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

all: audit test ## Run the audit, then the full test suite

audit: vet fmt build ## Static checks only: vet + gofmt + build (no tests)

test: clean ## Run every test verbosely with a freshly cleared cache
	$(GO) test -v ./...

test-race: clean ## Run every test under the race detector with a freshly cleared cache
	$(GO) test -race ./...

cover: clean ## Run all tests with coverage; per-package output and the total
	$(GO) test -cover -coverprofile=$(COVER) ./...
	@$(GO) tool cover -func=$(COVER) | awk 'END { print "total coverage: " $$NF }'

cover-html: cover ## Build an HTML coverage report (coverage.html) from the profile
	$(GO) tool cover -html=$(COVER) -o coverage.html
	@echo "wrote coverage.html"

vet: ## Run go vet over all packages
	$(GO) vet ./...

fmt: ## Report gofmt drift (fails if any file needs formatting)
	@files=$$(gofmt -l .); \
	if [ -n "$$files" ]; then \
		echo "gofmt: the following files are not formatted:"; \
		echo "$$files"; \
		exit 1; \
	fi; \
	echo "gofmt: clean"

fmt-fix: ## Reformat every file in place with gofmt
	gofmt -w .

build: ## Compile all packages (non-test code)
	$(GO) build ./...

tidy: ## Verify go.mod/go.sum are tidy
	$(GO) mod tidy -diff

clean: ## Drop the cached test results
	$(GO) clean -testcache
