// Package gen holds the code generated from api/openapi.yaml by oapi-codegen:
// the request/response models and a strict net/http server interface. The
// OpenAPI spec is the single source of truth — do not edit api.gen.go by hand.
//
// Regenerate after changing the spec with `make generate` (or `go generate
// ./...`). The oapi-codegen version is pinned via the tool directive in go.mod
// (Dependabot bumps it), and CI regenerates and fails if the output drifts, so
// the spec and generated code stay in lock-step.
package gen

//go:generate go tool oapi-codegen -config cfg.yaml ../../../api/openapi.yaml
