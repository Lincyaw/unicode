.PHONY: check lint test fmt-check self-check rule-check api api-check deep

# check is what a lane runs before it commits: fast, default rapid settings.
check: lint test fmt-check self-check rule-check api-check

lint:
	golangci-lint run

test:
	go test ./...

fmt-check:
	go run ./cmd/unicode fmt --check

# The toolchain checks itself: its design, its code and the bindings between them.
self-check:
	go run ./cmd/unicode check

# Every numbered rule of docs/language.md and docs/model.md names the codes
# that report it broken, and the tables list exactly the codes of the source.
rule-check:
	python3 scripts/rulecodes.py

# The viewer's interface (docs/design.md §7.5): api/openapi.yaml is generated
# from api/*.tsp. api regenerates it; api-check fails when it differs.
api:
	pnpm --dir api install --frozen-lockfile --silent
	pnpm --dir api run --silent build

api-check:
	pnpm --dir api install --frozen-lockfile --silent
	pnpm --dir api run --silent check

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
