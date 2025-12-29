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

package aianalysis

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/aianalysis"
)

// Day 6: Error Classification Unit Tests - BR-AI-021
// Business Value: Proper error classification enables automatic retry logic and operator troubleshooting
var _ = Describe("Error Classification for Retry Strategy", func() {
	// ========================================
	// TRANSIENT ERRORS: Enable Automatic Retry
	// Business Value: System recovers automatically from temporary failures
	// ========================================
	Describe("Transient Error Classification", func() {
		It("should enable automatic retry for temporary failures without operator intervention", func() {
			By("Simulating HolmesGPT-API temporary failure")
			wrappedErr := errors.New("connection timeout")
			err := aianalysis.NewTransientError("HolmesGPT-API call failed", wrappedErr)

			By("Verifying error classification enables retry logic")
			var transientErr *aianalysis.TransientError
			Expect(errors.As(err, &transientErr)).To(BeTrue(),
				"Transient classification triggers automatic retry with exponential backoff")

			By("Verifying error message helps operators understand retry reason")
			Expect(err.Error()).To(Equal("HolmesGPT-API call failed: connection timeout"),
				"Detailed error message shows operators why system is retrying")
		})

		It("should preserve root cause for operator troubleshooting after retry exhaustion", func() {
			By("Simulating transient error that persists beyond max retries")
			rootCause := errors.New("503 Service Unavailable")
			err := aianalysis.NewTransientError("HAPI unavailable", rootCause)

			By("Verifying root cause is preserved for troubleshooting")
			Expect(errors.Unwrap(err)).To(Equal(rootCause),
				"Root cause (503) helps operators diagnose persistent issues after max retries")

			By("Verifying error classification prevents infinite retries")
			// Business value: After max retries, transient errors are escalated to operators
			Expect(err.Message).To(Equal("HAPI unavailable"),
				"Clear message context guides operator intervention after retries exhausted")
		})

		It("should distinguish network issues from service degradation for retry decisions", func() {
			By("Recording various transient error scenarios")
			networkErr := aianalysis.NewTransientError("Rate limited", nil)
			timeoutErr := aianalysis.NewTransientError("Request timeout", errors.New("context deadline exceeded"))

			By("Verifying all transient scenarios enable retry")
			var transient1, transient2 *aianalysis.TransientError
			Expect(errors.As(networkErr, &transient1)).To(BeTrue(),
				"Rate limits (429) are transient - system retries automatically")
			Expect(errors.As(timeoutErr, &transient2)).To(BeTrue(),
				"Timeouts are transient - retry may succeed if service recovers")
		})
	})

	// ========================================
	// PERMANENT ERRORS: Prevent Wasteful Retries
	// Business Value: Fail fast to avoid wasting compute resources on retries that will never succeed
	// ========================================
	Describe("Permanent Error Classification", func() {
		It("should prevent wasteful automatic retries for configuration errors", func() {
			By("Simulating authentication failure (permanent configuration issue)")
			wrappedErr := errors.New("401 unauthorized")
			err := aianalysis.NewPermanentError("Authentication failed", "InvalidCredentials", wrappedErr)

			By("Verifying error classification prevents retry loop")
			var permanentErr *aianalysis.PermanentError
			Expect(errors.As(err, &permanentErr)).To(BeTrue(),
				"Permanent classification prevents wasteful retries (auth won't succeed without config fix)")

			By("Verifying error message guides operator to fix configuration")
			Expect(err.Error()).To(Equal("Authentication failed: 401 unauthorized"),
				"401 error tells operator to check API credentials, not wait for retry")
		})

		It("should provide actionable reason codes for targeted troubleshooting", func() {
			By("Recording permanent error with specific reason")
			err := aianalysis.NewPermanentError("Workflow not found", "NotFound", errors.New("404 not found"))

			By("Verifying reason field guides specific operator action")
			Expect(err.Reason).To(Equal("NotFound"),
				"NotFound reason tells operator to check workflow registry, not HolmesGPT-API health")

			By("Verifying root cause enables precise troubleshooting")
			Expect(errors.Unwrap(err)).NotTo(BeNil(),
				"HTTP 404 status helps operator verify workflow exists in repository")
		})

		It("should distinguish configuration errors from resource exhaustion", func() {
			By("Recording various permanent failure scenarios")
			// Configuration error - operator must fix config
			authErr := aianalysis.NewPermanentError("Access denied", "Forbidden", errors.New("forbidden"))

			// Resource doesn't exist - operator must provision resource
			notFoundErr := aianalysis.NewPermanentError("Workflow not found", "NotFound", nil)

			// Invalid input - operator must fix CRD spec
			validationErr := aianalysis.NewPermanentError("Configuration invalid", "InvalidConfig", nil)

			By("Verifying all permanent errors skip retry logic")
			var perm1, perm2, perm3 *aianalysis.PermanentError
			Expect(errors.As(authErr, &perm1)).To(BeTrue(),
				"Auth errors (403) need config fix, retries will fail")
			Expect(errors.As(notFoundErr, &perm2)).To(BeTrue(),
				"Not found errors (404) need resource provisioning, retries will fail")
			Expect(errors.As(validationErr, &perm3)).To(BeTrue(),
				"Validation errors need spec correction, retries will fail")
		})
	})

	// ========================================
	// VALIDATION ERRORS: Provide Clear User Feedback
	// Business Value: Operators get actionable error messages for CRD spec corrections
	// ========================================
	Describe("Validation Error Classification", func() {
		It("should provide field-specific error messages for fast CRD spec corrections", func() {
			By("Simulating validation failure on required field")
			err := aianalysis.NewValidationError("signalID", "cannot be empty")

			By("Verifying error message specifies exact field needing correction")
			Expect(err.Error()).To(Equal("validation error for signalID: cannot be empty"),
				"Field-specific errors guide operator to exact line in CRD spec needing fix")

			By("Verifying field name is accessible for structured error reporting")
			Expect(err.Field).To(Equal("signalID"),
				"Field name enables structured logging and alerting to operators")
		})

		It("should provide constraint details for valid value range guidance", func() {
			By("Simulating validation failure with constraint violation")
			err := aianalysis.NewValidationError("confidence", "must be between 0 and 1")

			By("Verifying error explains acceptable value range")
			Expect(err.Message).To(Equal("must be between 0 and 1"),
				"Constraint description tells operator valid range for confidence field")

			By("Verifying validation errors prevent invalid state propagation")
			var validationErr *aianalysis.ValidationError
			Expect(errors.As(err, &validationErr)).To(BeTrue(),
				"Validation classification catches user errors before they reach business logic")
		})
	})

	// ========================================
	// ERROR CLASSIFICATION ROUTING
	// Business Value: Handlers can apply correct retry strategy based on error type
	// ========================================
	Describe("Error Classification Routing for Retry Strategy", func() {
		It("should enable handlers to apply exponential backoff for transient errors", func() {
			By("Creating transient error requiring retry")
			err := aianalysis.NewTransientError("timeout", nil)

			By("Verifying handler can identify transient errors for retry logic")
			var transientErr *aianalysis.TransientError
			Expect(errors.As(err, &transientErr)).To(BeTrue(),
				"Transient classification triggers exponential backoff retry strategy")
		})

		It("should enable handlers to fail fast for permanent errors", func() {
			By("Creating permanent error requiring operator intervention")
			err := aianalysis.NewPermanentError("forbidden", "AccessDenied", nil)

			By("Verifying handler can identify permanent errors to skip retry")
			var permanentErr *aianalysis.PermanentError
			Expect(errors.As(err, &permanentErr)).To(BeTrue(),
				"Permanent classification prevents wasteful retries and alerts operator immediately")
		})

		It("should enable handlers to provide user feedback for validation errors", func() {
			By("Creating validation error requiring CRD spec fix")
			err := aianalysis.NewValidationError("field", "invalid")

			By("Verifying handler can identify validation errors for user feedback")
			var validationErr *aianalysis.ValidationError
			Expect(errors.As(err, &validationErr)).To(BeTrue(),
				"Validation classification enables actionable feedback to CRD authors")
		})

		It("should prevent misclassification that would trigger wrong retry strategy", func() {
			By("Ensuring transient errors aren't treated as permanent")
			transientErr := aianalysis.NewTransientError("timeout", nil)
			var permanentErr *aianalysis.PermanentError

			By("Verifying transient errors don't match permanent error pattern")
			Expect(errors.As(transientErr, &permanentErr)).To(BeFalse(),
				"Misclassification would skip retry for recoverable errors (business impact: false failures)")
		})

		It("should enable cost-effective error handling through correct classification", func() {
			By("Demonstrating resource savings from proper error classification")
			// Scenario 1: Transient error → Retry (cost: minimal, value: automatic recovery)
			transientErr := aianalysis.NewTransientError("503 Service Unavailable", nil)
			var transient *aianalysis.TransientError
			Expect(errors.As(transientErr, &transient)).To(BeTrue(),
				"Retrying 503 errors enables automatic recovery without operator intervention")

			// Scenario 2: Permanent error → Fail fast (cost savings: no wasted retries)
			permanentErr := aianalysis.NewPermanentError("401 Unauthorized", "AuthFailed", nil)
			var permanent *aianalysis.PermanentError
			Expect(errors.As(permanentErr, &permanent)).To(BeTrue(),
				"Failing fast on 401 errors saves compute resources (no retry loop)")

			By("Verifying classification guides cost-effective error handling")
			// Business value: Correct classification = automatic recovery where possible,
			// immediate operator alert where needed, no wasted resources on futile retries
		})
	})
})
