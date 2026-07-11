GO            ?= go
GOLANGCI_LINT ?= golangci-lint

.PHONY: all help build test test-short test-race cover lint fmt fmt-check vet tidy check ci clean

all: check

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Compile all packages
	$(GO) build ./...

test: ## Run all tests
	$(GO) test ./...

test-short: ## Run only fast unit tests (skip DB-backed integration tests)
	$(GO) test -short ./...

test-race: ## Run tests with the race detector
	$(GO) test -race ./...

cover: ## Run tests and print coverage by function
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

lint: ## Run golangci-lint
	$(GOLANGCI_LINT) run

fmt: ## Apply formatters (gofmt + goimports)
	$(GOLANGCI_LINT) fmt

fmt-check: ## Fail if any file is not formatted
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "gofmt needed for:"; echo "$$out"; exit 1; fi

vet: ## Run go vet
	$(GO) vet ./...

tidy: ## Tidy go.mod / go.sum
	$(GO) mod tidy

check: fmt-check vet lint test-short ## Format check + vet + lint + fast tests (local gate)

ci: build vet lint test-race ## Full CI gate

clean: ## Remove build/coverage artifacts
	rm -f coverage.out
