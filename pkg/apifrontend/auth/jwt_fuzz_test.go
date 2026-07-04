/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package auth_test

import (
	"context"
	"testing"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

// FuzzJWTValidatorValidate exercises JWTValidator.Validate with adversarial
// token strings to surface panics or unrecovered errors in the JWT parsing
// path. This is the highest-value fuzz target in the repo: rawToken is the
// raw bearer token string taken directly from an incoming Authorization
// header on apifrontend's public HTTP API -- fully attacker-controlled.
//
// Constructed with zero providers and no TokenReview reviewer, so any
// parseable token immediately hits "unknown issuer" and returns without any
// network/JWKS call, keeping the target fully offline and deterministic.
// This still exercises the byte-level JWS parsing (josejwt.ParseSigned) and
// unverified-claim decode (extractIssuerUnsafe) ahead of any crypto check.
//
// NOTE: native Go fuzzing (func FuzzXxx(f *testing.F)) is the sole,
// documented exception to AGENTS.md's Ginkgo/Gomega mandate -- see
// "Exception: Go Native Fuzz Tests" in AGENTS.md. This file intentionally
// contains no business-outcome assertions.
//
// Run locally with: go test ./pkg/apifrontend/auth/ -run=^$ -fuzz=FuzzJWTValidatorValidate
func FuzzJWTValidatorValidate(f *testing.F) {
	seeds := []string{
		"eyJhbGciOiAiUlMyNTYiLCAidHlwIjogIkpXVCJ9.eyJpc3MiOiAiaHR0cHM6Ly9leGFtcGxlLmNvbSIsICJzdWIiOiAidXNlcjEiLCAiZXhwIjogOTk5OTk5OTk5OX0.ZmFrZXNpZ25hdHVyZWJ5dGVz",
		"",
		"not.a.jwt",
		"..",
		"Bearer garbage",
		"eyJhbGciOiJub25lIn0.e30.",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	validator, err := auth.NewJWTValidator(auth.Config{})
	if err != nil {
		f.Fatalf("failed to construct JWTValidator with empty config: %v", err)
	}
	ctx := context.Background()

	f.Fuzz(func(t *testing.T, rawToken string) {
		// The only contract under test: Validate must never panic, regardless
		// of whether it accepts or rejects the token.
		_, _ = validator.Validate(ctx, rawToken)
	})
}
