.PHONY: check lint test fmt-check self-check deep

# check is what a lane runs before it commits: fast, default rapid settings.
check: lint test fmt-check self-check

lint:
	golangci-lint run

test:
	go test ./...

fmt-check:
	go run ./cmd/unicode fmt --check

# The toolchain checks its own design (M2 extends this to the code).
self-check:
	go run ./cmd/unicode check --design-only

# deep is the asynchronous stage after a merge (CLAUDE.md principle 9):
# many rapid checks, a fuzz burst per Fuzz target, and mutation testing.
RAPID_CHECKS ?= 5000
FUZZ_TIME ?= 30s

deep:
	go test ./... -count=1 -rapid.checks=$(RAPID_CHECKS)
	./scripts/fuzz-all.sh $(FUZZ_TIME)
	# gremlins takes a directory, not a package pattern; run it from the root,
	# uncached, with a timeout that covers recompiling a mutant.
	GOFLAGS=-count=1 go tool gremlins unleash --timeout-coefficient 30
