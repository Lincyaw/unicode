.PHONY: check lint test fmt-check gen-check self-check rule-check api api-check serve-gen serve-gen-check deep

# check is what a lane runs before it commits: fast, default rapid settings.
check: lint fmt-check gen-check self-check rule-check api-check serve-gen-check
	$(MAKE) test

lint:
	golangci-lint run

test:
	go test -timeout=20m ./...

fmt-check:
	go run ./cmd/unicode fmt --check

# Generated tests must be current before any expensive property execution.
gen-check:
	go run ./cmd/unicode gen --check

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

# The server's Go code (docs/design.md §3) is generated from api/openapi.yaml by
# oapi-codegen. serve-gen regenerates it; serve-gen-check fails when it differs.
SERVE_GEN = go tool oapi-codegen -config internal/serve/oapi-codegen.yaml api/openapi.yaml

serve-gen:
	$(SERVE_GEN) > internal/serve/openapi.gen.go.tmp
	mv internal/serve/openapi.gen.go.tmp internal/serve/openapi.gen.go

serve-gen-check:
	@$(SERVE_GEN) | cmp -s - internal/serve/openapi.gen.go || { echo "internal/serve/openapi.gen.go differs from what oapi-codegen generates from api/openapi.yaml; it is generated, so do not edit it: run make serve-gen" >&2; exit 1; }

# deep is the asynchronous stage after a merge (CLAUDE.md principle 9):
# many rapid checks, a fuzz burst per Fuzz target, and mutation testing.
RAPID_CHECKS ?= 5000
FUZZ_TIME ?= 30s
DEEP_TIMEOUT ?= 0

deep:
	go test ./... -timeout=$(DEEP_TIMEOUT) -count=1 -rapid.checks=$(RAPID_CHECKS)
	./scripts/fuzz-all.sh $(FUZZ_TIME)
	# gremlins takes a directory, not a package pattern; run it from the root,
	# uncached, with a timeout that covers recompiling a mutant.
	GOFLAGS=-count=1 go tool gremlins unleash --timeout-coefficient 30
