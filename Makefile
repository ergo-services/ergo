# Makefile for the ergo.services/ergo core module.
# Runs the full test suite and a static audit (vet, gofmt, build) over every package.

GO ?= go
COVER ?= coverage.out

.PHONY: help all audit test test-race bench bench-stat cover cover-html vet fmt fmt-fix build tidy clean

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

all: audit test ## Run the audit, then the full test suite

audit: vet fmt build ## Static checks only: vet + gofmt + build (no tests)

test: clean ## Run every test verbosely with a freshly cleared cache
	$(GO) test -v ./...

test-race: clean ## Run every test under the race detector with a freshly cleared cache
	$(GO) test -race ./...

# A duration gives every scenario the same window on any machine. A fixed
# message count would be tuned to one core count.
BENCHTIME ?= 5s

bench: ## Run the benchmarks (nothing else runs them; override BENCHTIME, e.g. BENCHTIME=14000000x for an exact message count)
	$(GO) test -run XXX -bench . -benchmem -benchtime $(BENCHTIME) ./testing/benchmarks/...

# BENCH_OUT keeps the raw samples for a later comparison; unset, they go to a
# temporary file and are removed. BENCH_BASE is such a file from an earlier run.
BENCHCOUNT ?= 6
BENCH_OUT ?=
BENCH_BASE ?=

bench-stat: ## Run the benchmarks BENCHCOUNT times and summarise with benchstat; BENCH_OUT=file keeps the samples, BENCH_BASE=file compares against them
	@bs=$$(command -v benchstat || echo "$$($(GO) env GOPATH)/bin/benchstat"); \
	if [ ! -x "$$bs" ]; then \
		echo "benchstat is not installed. Get it with:"; \
		echo ""; \
		echo "    go install golang.org/x/perf/cmd/benchstat@latest"; \
		echo ""; \
		echo "and make sure $$($(GO) env GOPATH)/bin is on your PATH."; \
		exit 1; \
	fi; \
	out="$(BENCH_OUT)"; keep=yes; \
	if [ -z "$$out" ]; then out=$$(mktemp); keep=no; fi; \
	echo "running $(BENCHCOUNT) rounds of $(BENCHTIME) per benchmark, this takes a while..."; \
	$(GO) test -run XXX -bench . -benchmem -benchtime $(BENCHTIME) -count $(BENCHCOUNT) \
		./testing/benchmarks/... | tee "$$out"; \
	status=0; \
	if grep -q '^Benchmark' "$$out"; then \
		echo ""; \
		if [ -n "$(BENCH_BASE)" ]; then \
			"$$bs" $(BENCH_BASE) "$$out"; \
		else \
			"$$bs" "$$out"; \
		fi; \
	else \
		echo "no benchmark results were produced"; \
		status=1; \
	fi; \
	if [ "$$keep" = "no" ]; then rm -f "$$out"; else echo ""; echo "samples kept in $$out"; fi; \
	exit $$status

cover: clean ## Run every test (incl. integration) measuring coverage of all non-testing packages; print only the total
	@echo "coverage: running all tests (incl. integration), this takes a bit..."
	@pkgs=$$($(GO) list ./... | grep -v '/testing/' | paste -sd, -); \
	log=$$(mktemp); \
	$(GO) test -coverpkg="$$pkgs" -coverprofile=$(COVER) ./... >"$$log" 2>&1; ec=$$?; \
	if [ $$ec -ne 0 ]; then cat "$$log"; rm -f "$$log"; exit $$ec; fi; \
	rm -f "$$log"; \
	$(GO) tool cover -func=$(COVER) | awk 'END { print "total coverage: " $$NF }'

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
	@$(GO) clean -testcache
