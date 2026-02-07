// Package logs provides a thin zerolog wrapper with optional colorized
// console output and a bypass mode for direct zerolog performance behavior.
//
// The package intentionally limits itself to logger configuration and simple
// helpers; transport and sink concerns should be handled by the configured
// io.Writer.
//
// Relevant docs (project integration):
// - docs/architecture/definitions/observability.toml
//
// - docs/glossary/observability.md
//
// - docs/progress/index.md
package logs
