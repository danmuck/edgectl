// Package docker owns the Docker container runtime seed adapter.
//
// Ownership boundary:
// - docker CLI command dispatch
//
// - container lifecycle operations (status, ps, run, stop, rm, logs, inspect)
//
// Canonical references (consult before changes):
// - docs/architecture/definitions/design.toml seed_examples["runtime.docker"]
//
// - docs/architecture/control-loop.md
//
// - docs/glossary/definitions.md
package docker
