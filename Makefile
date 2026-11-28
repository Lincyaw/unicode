.PHONY: check check-steps build lint test fmt-check gen-check self-check rule-check api api-check serve-gen serve-gen-check web-check deep

# check is what a lane runs once its work is done: default rapid settings. Full
# runs on one machine take turns through one lock (CLAUDE.md, Roles and
# workflow), so overlapping runs neither slow each other nor skew time budgets;
# where flock is missing, the run goes ahead unlocked.
CHECK_LOCK ?= /tmp/unicode-make-check-$(shell id -u).lock

check:
	@if [ -z "$$UNICODE_CHECK_LOCKED" ] && command -v flock >/dev/null 2>&1; then \
		UNICODE_CHECK_LOCKED=1 flock "$(CHECK_LOCK)" $(MAKE) check-steps; \
	else \
		$(MAKE) check-steps; \
	fi

check-steps: lint fmt-check gen-check self-check rule-check api-check serve-gen-check web-check
	$(MAKE) test

# build makes bin/unicode with the viewer's pages in it (docs/design.md §8): the
# frontend's build writes them to internal/serve/ui/dist, which go:embed takes in.
# unicode --version prints VERSION, by default what git describe gives.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null)

build:
	pnpm --dir web install --frozen-lockfile
	pnpm --dir web build
	go build -ldflags "-X main.version=$(VERSION)" -o bin/unicode ./cmd/unicode

lint:
	golangci-lint run --allow-serial-runners

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
# that report it broken, the tables list exactly the codes of the source, and
# every string the source writes where a code is reported or compared, or that
# starts with its component's name and a dot, is a code that some component's
# source reports, of the form <component>.<kebab-name>, reported only by that
# component.
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

# The viewer's frontend (docs/design.md §7.5): web/ generates its types from
# api/openapi.yaml, then typechecks, lints its code and styles, and builds.
web-check:
	pnpm --dir web install --frozen-lockfile --silent
	pnpm --dir web run --silent check

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
