package auth

import (
	"errors"
	"testing"

	logs "github.com/danmuck/smplog"
)

func TestStaticTokenValidate(t *testing.T) {
	tests := []struct {
		name    string
		stored  string
		input   string
		wantErr error
	}{
		{name: "empty token denied", stored: "", input: "abc", wantErr: ErrUnauthorized},
		{name: "mismatched token denied", stored: "abc", input: "xyz", wantErr: ErrUnauthorized},
		{name: "matching token accepted", stored: "abc", input: "abc", wantErr: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logs.Logf("auth/static-token: stored=%q input=%q", tc.stored, tc.input)
			err := (StaticToken{Token: tc.stored}).Validate(tc.input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected err %v, got %v", tc.wantErr, err)
			}
			logs.Logf("auth/static-token: result err=%v", err)
		})
	}
}

func TestFuncValidator(t *testing.T) {
	validator := FuncValidator(func(token string) error {
		logs.Logf("auth/func-validator: validating token=%q", token)
		if token != "ok" {
			return ErrUnauthorized
		}
		return nil
	})

	if err := validator.Validate("bad"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected unauthorized for bad token, got %v", err)
	}
	if err := validator.Validate("ok"); err != nil {
		t.Fatalf("expected success for ok token, got %v", err)
	}
	logs.Logf("auth/func-validator: path complete")
}
