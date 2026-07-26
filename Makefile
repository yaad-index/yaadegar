.PHONY: help build test vet lint fmt generate verify-generate check clean

help:
	@echo "make build           — compile yaadegar to ./yaadegar"
	@echo "make test            — race-mode unit tests"
	@echo "make vet             — go vet"
	@echo "make lint            — golangci-lint run"
	@echo "make fmt             — gofumpt -w ."
	@echo "make generate        — regenerate code from api/openapi.yaml (oapi-codegen)"
	@echo "make verify-generate — fail if generated code is stale vs the spec"
	@echo "make check           — vet + test + lint"
	@echo "make clean           — remove built binaries"

build:
	go build -o yaadegar ./cmd/yaadegar

# generate regenerates code from the OpenAPI spec. oapi-codegen is pinned via the
# tool directive in go.mod, so the output is reproducible (Dependabot bumps it).
generate:
	go generate ./...

# verify-generate regenerates and fails if the checked-in output changed — the
# spec and the generated code must stay in lock-step. CI runs this in the check
# job (where the generated file is committed, so any drift shows as a diff).
verify-generate: generate
	@git --no-pager diff --exit-code -- internal/api/gen || \
		{ echo "generated code is out of date; run 'make generate' and commit."; exit 1; }

test:
	go test -race -timeout 2m ./...

vet:
	go vet ./...

lint:
	golangci-lint run

fmt:
	gofumpt -w .

check: vet test lint

clean:
	rm -f yaadegar
