// Package auth provides minimal authentication helpers.
//
// It intentionally avoids policy decisions and storage concerns.
package auth

import (
	"crypto/subtle"
	"errors"
)

var ErrUnauthorized = errors.New("auth: unauthorized")

// Validator validates an authentication token.
type Validator interface {
	Validate(token string) error
}

// StaticToken is a simple validator for a single shared token.
// It is intended only for development and proofs of concept.
type StaticToken struct {
	Token string
}

func (s StaticToken) Validate(token string) error {
	if s.Token == "" {
		return ErrUnauthorized
	}
	if subtle.ConstantTimeCompare([]byte(s.Token), []byte(token)) != 1 {
		return ErrUnauthorized
	}
	return nil
}

// FuncValidator adapts a function into a Validator.
type FuncValidator func(token string) error

func (f FuncValidator) Validate(token string) error {
	return f(token)
}
