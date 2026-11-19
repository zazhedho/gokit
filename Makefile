GOLANGCI_LINT ?= $(shell go env GOPATH)/bin/golangci-lint
GOLANGCI_LINT_VERSION := v2.14.0

.PHONY: lint lint-install test vet cover

lint:
	@echo "Running golangci-lint..."
	@if [ ! -x "$(GOLANGCI_LINT)" ]; then \
		echo "golangci-lint is not installed. Run: make lint-install"; \
		exit 1; \
	fi
	@$(GOLANGCI_LINT) run --config .golangci.yml --timeout=5m ./...
	@echo "golangci-lint passed."

lint-install:
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

test:
	@go test -race -count=1 ./...

vet:
	@go vet ./...

cover:
	@go test -race -count=1 -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out | tail -n 1
