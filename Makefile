.PHONY: check lint test fmt-check

check: lint test fmt-check

lint:
	golangci-lint run

test:
	go test ./...

fmt-check:
	go run ./cmd/unicode fmt --check
