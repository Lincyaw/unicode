.PHONY: check lint test fmt-check deep

# check is what a lane runs before it commits: fast, default rapid settings.
check: lint test fmt-check

lint:
	golangci-lint run

test:
	go test ./...

fmt-check:
	go run ./cmd/unicode fmt --check

# deep is the asynchronous stage after a merge (CLAUDE.md principle 9):
# many rapid checks, a fuzz burst per Fuzz target, and mutation testing.
RAPID_CHECKS ?= 5000
FUZZ_TIME ?= 30s

deep:
	go test ./... -count=1 -rapid.checks=$(RAPID_CHECKS)
	./scripts/fuzz-all.sh $(FUZZ_TIME)
	go tool gremlins unleash ./...
