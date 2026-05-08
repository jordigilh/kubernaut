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

package audit

import (
	"errors"
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/audit"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// AUDIT ERROR TYPES UNIT TESTS (GAP-11)
// 📋 Design Decision: DD-AUDIT-002 | BR-AUDIT-001
// Defense-in-Depth: Tier 2 - Unit Tests for Error Types
// Tier 1: test/unit/audit/http_client_test.go
// ========================================
//
// These tests verify the error type behavior for retry logic:
// - HTTPError: 4xx = NOT retryable, 5xx = retryable
// - NetworkError: Always retryable
// - MarshalError: Never retryable
//
// This ensures BufferedAuditStore correctly differentiates errors for:
// - DLQ routing (4xx errors)
// - Retry with backoff (5xx and network errors)
//
// ========================================

var _ = Describe("Audit Error Types (GAP-11)", func() {
	Context("HTTPError", func() {
		// BEHAVIOR: 4xx errors are NOT retryable (client errors)
		// CORRECTNESS: Invalid data should not be retried
		It("should NOT be retryable for 400 Bad Request", func() {
			err := audit.NewHTTPError(400, "Bad Request")

			Expect(err.IsRetryable()).To(BeFalse(), "400 errors should NOT be retryable")
			Expect(err.Is4xxError()).To(BeTrue())
			Expect(err.Is5xxError()).To(BeFalse())
		})

		// UT-AE-1056-001: 401 is retryable (auth errors are transient when using file-based token rotation)
		It("UT-AE-1056-001: should be retryable for 401 Unauthorized", func() {
			err := audit.NewHTTPError(401, "Unauthorized")

			Expect(err.IsRetryable()).To(BeTrue(), "401 auth errors should be retryable (#1056)")
			Expect(err.Is4xxError()).To(BeTrue())
			Expect(err.IsAuthError()).To(BeTrue())
		})

		// UT-AE-1056-002: 403 is retryable (auth errors are transient when using file-based token rotation)
		It("UT-AE-1056-002: should be retryable for 403 Forbidden", func() {
			err := audit.NewHTTPError(403, "Forbidden")

			Expect(err.IsRetryable()).To(BeTrue(), "403 auth errors should be retryable (#1056)")
			Expect(err.Is4xxError()).To(BeTrue())
			Expect(err.IsAuthError()).To(BeTrue())
		})

		It("should NOT be retryable for 404 Not Found", func() {
			err := audit.NewHTTPError(404, "Not Found")

			Expect(err.IsRetryable()).To(BeFalse())
			Expect(err.Is4xxError()).To(BeTrue())
		})

		It("should NOT be retryable for 422 Unprocessable Entity", func() {
			err := audit.NewHTTPError(422, "Unprocessable Entity")

			Expect(err.IsRetryable()).To(BeFalse())
			Expect(err.Is4xxError()).To(BeTrue())
		})

		// BEHAVIOR: 5xx errors ARE retryable (server errors)
		// CORRECTNESS: Temporary server failures should be retried
		It("should be retryable for 500 Internal Server Error", func() {
			err := audit.NewHTTPError(500, "Internal Server Error")

			Expect(err.IsRetryable()).To(BeTrue(), "500 errors SHOULD be retryable")
			Expect(err.Is5xxError()).To(BeTrue())
			Expect(err.Is4xxError()).To(BeFalse())
		})

		It("should be retryable for 502 Bad Gateway", func() {
			err := audit.NewHTTPError(502, "Bad Gateway")

			Expect(err.IsRetryable()).To(BeTrue())
			Expect(err.Is5xxError()).To(BeTrue())
		})

		It("should be retryable for 503 Service Unavailable", func() {
			err := audit.NewHTTPError(503, "Service Unavailable")

			Expect(err.IsRetryable()).To(BeTrue())
			Expect(err.Is5xxError()).To(BeTrue())
		})

		It("should be retryable for 504 Gateway Timeout", func() {
			err := audit.NewHTTPError(504, "Gateway Timeout")

			Expect(err.IsRetryable()).To(BeTrue())
			Expect(err.Is5xxError()).To(BeTrue())
		})

		It("should include status code in error message", func() {
			err := audit.NewHTTPError(503, "Service Unavailable")

			Expect(err.Error()).To(ContainSubstring("503"))
			Expect(err.Error()).To(ContainSubstring("Service Unavailable"))
		})
	})

	Context("NetworkError", func() {
		// BEHAVIOR: Network errors are ALWAYS retryable
		// CORRECTNESS: Connection failures are temporary
		It("should always be retryable", func() {
			underlying := errors.New("connection refused")
			err := audit.NewNetworkError(underlying)

			Expect(err.IsRetryable()).To(BeTrue(), "Network errors should ALWAYS be retryable")
		})

		It("should wrap the underlying error", func() {
			underlying := errors.New("connection timeout")
			err := audit.NewNetworkError(underlying)

			Expect(errors.Unwrap(err)).To(Equal(underlying))
		})

		It("should include underlying error in message", func() {
			underlying := errors.New("dial tcp: connection refused")
			err := audit.NewNetworkError(underlying)

			Expect(err.Error()).To(ContainSubstring("connection refused"))
			Expect(err.Error()).To(ContainSubstring("network error"))
		})
	})

	Context("MarshalError", func() {
		// BEHAVIOR: Marshal errors are NEVER retryable
		// CORRECTNESS: Code bugs cannot be fixed by retry
		It("should never be retryable", func() {
			underlying := errors.New("json: unsupported type")
			err := audit.NewMarshalError(underlying)

			Expect(err.IsRetryable()).To(BeFalse(), "Marshal errors should NEVER be retryable")
		})

		It("should wrap the underlying error", func() {
			underlying := errors.New("json: unsupported type: chan int")
			err := audit.NewMarshalError(underlying)

			Expect(errors.Unwrap(err)).To(Equal(underlying))
		})

		It("should include underlying error in message", func() {
			underlying := errors.New("json: unsupported type")
			err := audit.NewMarshalError(underlying)

			Expect(err.Error()).To(ContainSubstring("unsupported type"))
			Expect(err.Error()).To(ContainSubstring("marshal"))
		})
	})

	Context("IsRetryable helper function", func() {
		// BEHAVIOR: Helper correctly identifies retryable errors
		// CORRECTNESS: Centralizes retry logic decision
		It("should return false for nil error", func() {
			Expect(audit.IsRetryable(nil)).To(BeFalse())
		})

		It("should return true for 5xx HTTPError", func() {
			err := audit.NewHTTPError(500, "Internal Server Error")
			Expect(audit.IsRetryable(err)).To(BeTrue())
		})

		It("should return false for 4xx HTTPError", func() {
			err := audit.NewHTTPError(400, "Bad Request")
			Expect(audit.IsRetryable(err)).To(BeFalse())
		})

		It("should return true for NetworkError", func() {
			err := audit.NewNetworkError(errors.New("timeout"))
			Expect(audit.IsRetryable(err)).To(BeTrue())
		})

		It("should return false for MarshalError", func() {
			err := audit.NewMarshalError(errors.New("json error"))
			Expect(audit.IsRetryable(err)).To(BeFalse())
		})

		It("should return true for unknown errors (fail-safe)", func() {
			err := fmt.Errorf("unknown error")
			Expect(audit.IsRetryable(err)).To(BeTrue(), "Unknown errors should be retryable (fail-safe)")
		})

		It("should return true for wrapped retryable errors", func() {
			innerErr := audit.NewHTTPError(500, "Internal Server Error")
			wrappedErr := fmt.Errorf("context: %w", innerErr)

			Expect(audit.IsRetryable(wrappedErr)).To(BeTrue())
		})

		It("should return false for wrapped non-retryable errors", func() {
			innerErr := audit.NewHTTPError(400, "Bad Request")
			wrappedErr := fmt.Errorf("context: %w", innerErr)

			Expect(audit.IsRetryable(wrappedErr)).To(BeFalse())
		})
	})

	Context("Is4xxError helper function", func() {
		It("should return true for 4xx HTTPError", func() {
			err := audit.NewHTTPError(400, "Bad Request")
			Expect(audit.Is4xxError(err)).To(BeTrue())
		})

		It("should return false for 5xx HTTPError", func() {
			err := audit.NewHTTPError(500, "Internal Server Error")
			Expect(audit.Is4xxError(err)).To(BeFalse())
		})

		It("should return false for non-HTTP errors", func() {
			err := audit.NewNetworkError(errors.New("timeout"))
			Expect(audit.Is4xxError(err)).To(BeFalse())
		})

		It("should return true for wrapped 4xx errors", func() {
			innerErr := audit.NewHTTPError(422, "Unprocessable Entity")
			wrappedErr := fmt.Errorf("validation failed: %w", innerErr)

			Expect(audit.Is4xxError(wrappedErr)).To(BeTrue())
		})
	})

	Context("Is5xxError helper function", func() {
		It("should return true for 5xx HTTPError", func() {
			err := audit.NewHTTPError(500, "Internal Server Error")
			Expect(audit.Is5xxError(err)).To(BeTrue())
		})

		It("should return false for 4xx HTTPError", func() {
			err := audit.NewHTTPError(400, "Bad Request")
			Expect(audit.Is5xxError(err)).To(BeFalse())
		})

		It("should return false for non-HTTP errors", func() {
			err := audit.NewNetworkError(errors.New("timeout"))
			Expect(audit.Is5xxError(err)).To(BeFalse())
		})

		It("should return true for wrapped 5xx errors", func() {
			innerErr := audit.NewHTTPError(503, "Service Unavailable")
			wrappedErr := fmt.Errorf("server error: %w", innerErr)

			Expect(audit.Is5xxError(wrappedErr)).To(BeTrue())
		})
	})

	Context("IsAuthError (#1056)", func() {
		// UT-AE-1056-005: IsAuthError returns true for 401
		It("UT-AE-1056-005: should return true for 401 Unauthorized", func() {
			err := audit.NewHTTPError(401, "Unauthorized")
			Expect(err.IsAuthError()).To(BeTrue())
		})

		// UT-AE-1056-006: IsAuthError returns true for 403
		It("UT-AE-1056-006: should return true for 403 Forbidden", func() {
			err := audit.NewHTTPError(403, "Forbidden")
			Expect(err.IsAuthError()).To(BeTrue())
		})

		// UT-AE-1056-007: IsAuthError returns false for 400
		It("UT-AE-1056-007: should return false for 400 Bad Request", func() {
			err := audit.NewHTTPError(400, "Bad Request")
			Expect(err.IsAuthError()).To(BeFalse())
		})

		// UT-AE-1056-008: IsAuthError returns false for 500
		It("UT-AE-1056-008: should return false for 500 Internal Server Error", func() {
			err := audit.NewHTTPError(500, "Internal Server Error")
			Expect(err.IsAuthError()).To(BeFalse())
		})
	})

	Context("IsAuthError package-level helper (#1056)", func() {
		// UT-AE-1056-009: Package-level IsAuthError with wrapped 401 error
		It("UT-AE-1056-009: should return true for wrapped 401 error", func() {
			innerErr := audit.NewHTTPError(401, "Unauthorized")
			wrappedErr := fmt.Errorf("DS call failed: %w", innerErr)

			Expect(audit.IsAuthError(wrappedErr)).To(BeTrue())
		})

		It("should return false for wrapped 400 error", func() {
			innerErr := audit.NewHTTPError(400, "Bad Request")
			wrappedErr := fmt.Errorf("DS call failed: %w", innerErr)

			Expect(audit.IsAuthError(wrappedErr)).To(BeFalse())
		})

		It("should return false for non-HTTP errors", func() {
			err := audit.NewNetworkError(errors.New("timeout"))
			Expect(audit.IsAuthError(err)).To(BeFalse())
		})

		It("should return false for nil error", func() {
			Expect(audit.IsAuthError(nil)).To(BeFalse())
		})

		// UT-AE-1056-003: Regression guard - 400 remains non-retryable
		It("UT-AE-1056-003: 400 should remain non-retryable", func() {
			err := audit.NewHTTPError(400, "Bad Request")
			Expect(err.IsRetryable()).To(BeFalse(), "400 must NOT be retryable")
		})

		// UT-AE-1056-004: Regression guard - 422 remains non-retryable
		It("UT-AE-1056-004: 422 should remain non-retryable", func() {
			err := audit.NewHTTPError(422, "Unprocessable Entity")
			Expect(err.IsRetryable()).To(BeFalse(), "422 must NOT be retryable")
		})
	})
})
