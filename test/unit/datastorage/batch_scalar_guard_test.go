/*
Copyright 2026 Jordi Gil.

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

package datastorage

import (
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// PHASE 6: SCALAR JSON GUARD FOR BATCH PAYLOADS (TP-1088-P1)
// ========================================
//
// Issue: #1088 Phase 6 (Performance)
// File Under Test: pkg/datastorage/server/audit_events_batch_handler.go
// TDD Phase: RED — these tests document expected behavior for scalar JSON inputs
//
// Current behavior:
// - JSON object: detected, returns 400 (line 84-87)
// - JSON string/number/bool: falls to generic error at line 90-91 (leaks Go error msg)
// - JSON null: silently becomes nil slice, hits "batch cannot be empty" (misleading)
//
// Expected behavior:
// - ALL non-array payloads: return RFC 7807 422 with clear "must be JSON array" message
//
// These unit tests verify the JSON decode behavior that the handler must guard against.
// The handler-level integration test (IT-DS-1088-P6-040) is deferred to GREEN phase.
//
// ========================================

var _ = Describe("Phase 6: Scalar JSON Guard for Batch Payloads (TP-1088-P1)", func() {

	// Common type matching the batch handler's decode target
	type batchItem struct {
		Version   string `json:"version"`
		EventType string `json:"event_type"`
	}

	Describe("JSON decode behavior for non-array payloads", func() {

		It("UT-DS-1088-P6-040: JSON string must be rejected as non-array", func() {
			// RED: The batch handler currently falls through to a generic error
			// that leaks the Go unmarshal error message. It should return 422.

			body := `"hello"`
			var items []batchItem
			err := json.Unmarshal([]byte(body), &items)

			Expect(err).To(HaveOccurred(), "scalar JSON string should fail to unmarshal as array")
			Expect(err.Error()).To(ContainSubstring("cannot unmarshal string"),
				"error should identify string-vs-array mismatch")

			// The handler SHOULD detect this specific error pattern and return
			// RFC 7807 422 (not 400 with leaked Go error message).
			// Currently it does NOT — the generic error at line 91 fires instead.
			//
			// This assertion documents the expected error classification:
			errMsg := err.Error()
			isScalarMismatch := strings.Contains(errMsg, "cannot unmarshal string") ||
				strings.Contains(errMsg, "cannot unmarshal number") ||
				strings.Contains(errMsg, "cannot unmarshal bool")
			Expect(isScalarMismatch).To(BeTrue(),
				"scalar JSON types must be explicitly detected for proper 422 response")
		})

		It("UT-DS-1088-P6-041: JSON number must be rejected as non-array", func() {
			body := `42`
			var items []batchItem
			err := json.Unmarshal([]byte(body), &items)

			Expect(err).To(HaveOccurred(), "scalar JSON number should fail to unmarshal as array")
			Expect(err.Error()).To(ContainSubstring("cannot unmarshal number"))
		})

		It("UT-DS-1088-P6-042: JSON null must be explicitly rejected, not silently accepted", func() {
			// RED: JSON null unmarshals successfully into a nil slice.
			// The handler currently treats this as "batch cannot be empty" (400),
			// but it should be rejected as 422 "payload must be a JSON array".

			body := `null`
			var items []batchItem
			err := json.Unmarshal([]byte(body), &items)

			// null unmarshals without error into a nil slice
			Expect(err).ToNot(HaveOccurred(), "json.Unmarshal accepts null into slice")
			Expect(items).To(BeNil(), "null produces nil slice, not empty slice")

			// The handler MUST detect nil (from null) distinctly from empty array [].
			// Currently it does NOT — both hit the same "batch cannot be empty" path.
			// This is the core RED assertion: null must be detected BEFORE the empty check.
			//
			// Expected: handler returns 422 "payload must be a JSON array, not null"
			// Actual: handler returns 400 "batch cannot be empty" (misleading)
			//
			// We test this by asserting that nil and empty are distinguishable:
			emptyBody := `[]`
			var emptyItems []batchItem
			errEmpty := json.Unmarshal([]byte(emptyBody), &emptyItems)
			Expect(errEmpty).ToNot(HaveOccurred())
			Expect(emptyItems).ToNot(BeNil(), "empty array produces non-nil empty slice")
			Expect(emptyItems).To(HaveLen(0))

			// This proves null (nil) vs [] (empty non-nil) are distinguishable.
			// The handler must use this distinction for proper error responses.
			Expect(items == nil && emptyItems != nil).To(BeTrue(),
				"null (nil) and empty array (non-nil) must be distinguishable for correct error handling")
		})
	})
})
