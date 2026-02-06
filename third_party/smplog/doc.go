// Package logs provides a thin zerolog wrapper with optional colorized
// console output and a bypass mode for direct zerolog performance behavior.
//
// The package intentionally limits itself to logger configuration and simple
// helpers; transport and sink concerns should be handled by the configured
// io.Writer.
package logs
