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

package validate_test

import (
	"testing"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
)

// FuzzRRID and FuzzAlertName exercise validate.RRID and validate.AlertName
// with adversarial strings to surface panics in the first line of defense
// before MCP tool-call arguments (rr_id, alert_name, etc. -- verbatim from
// an external AI agent/user invoking apifrontend's tools) are used to build
// Kubernetes API calls. RRID transitively exercises Namespace and
// ResourceName. Both targets are pure functions with no dependencies.
//
// NOTE: native Go fuzzing (func FuzzXxx(f *testing.F)) is the sole,
// documented exception to AGENTS.md's Ginkgo/Gomega mandate -- see
// "Exception: Go Native Fuzz Tests" in AGENTS.md. This file intentionally
// contains no business-outcome assertions.
//
// Run locally with: go test ./pkg/apifrontend/validate/ -run=^$ -fuzz=FuzzRRID
func FuzzRRID(f *testing.F) {
	seeds := []string{
		"production/api-server-1",
		"api-server-1",
		"",
		"/",
		"a/b/c",
		"-invalid-/also-invalid-",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, rrid string) {
		_ = validate.RRID(rrid)
	})
}

// Run locally with: go test ./pkg/apifrontend/validate/ -run=^$ -fuzz=FuzzAlertName
func FuzzAlertName(f *testing.F) {
	seeds := []string{
		"HighMemoryUsage",
		"",
		":::",
		"1StartsWithDigit",
		"valid_name:with_colons",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, name string) {
		_ = validate.AlertName(name)
	})
}
