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

// Day 6: Error Types Unit Tests
// ERROR_HANDLING_PHILOSOPHY.md: Validate error categorization and behavior
var _ = Describe("AIAnalysis Error Types", func() {
	// ========================================
	// TRANSIENT ERROR
	// ========================================
	Describe("TransientError", func() {
		// BR-AI-021: Transient errors should be retried
		// Business Value: Operators can distinguish retryable failures

		Context("Error() method", func() {
			It("should include wrapped error message when present", func() {
				wrappedErr := errors.New("connection timeout")
				err := aianalysis.NewTransientError("HolmesGPT-API call failed", wrappedErr)

				Expect(err.Error()).To(Equal("HolmesGPT-API call failed: connection timeout"))
			})

			It("should return message only when no wrapped error", func() {
				err := aianalysis.NewTransientError("Rate limited", nil)

				Expect(err.Error()).To(Equal("Rate limited"))
			})
		})

		Context("Unwrap() method", func() {
			It("should return wrapped error for error chain inspection", func() {
				wrappedErr := errors.New("503 Service Unavailable")
				err := aianalysis.NewTransientError("HAPI unavailable", wrappedErr)

				Expect(errors.Unwrap(err)).To(Equal(wrappedErr))
			})

			It("should return nil when no wrapped error", func() {
				err := aianalysis.NewTransientError("Timeout", nil)

				Expect(errors.Unwrap(err)).To(BeNil())
			})
		})

		Context("Constructor", func() {
			It("should create transient error with all fields", func() {
				wrappedErr := errors.New("network error")
				err := aianalysis.NewTransientError("API call failed", wrappedErr)

				Expect(err).NotTo(BeNil())
				Expect(err.Message).To(Equal("API call failed"))
				Expect(err.Err).To(Equal(wrappedErr))
			})
		})
	})

	// ========================================
	// PERMANENT ERROR
	// ========================================
	Describe("PermanentError", func() {
		// BR-AI-021: Permanent errors should NOT be retried
		// Business Value: Operators can skip futile retries

		Context("Error() method", func() {
			It("should include wrapped error message when present", func() {
				wrappedErr := errors.New("401 unauthorized")
				err := aianalysis.NewPermanentError("Authentication failed", "InvalidCredentials", wrappedErr)

				Expect(err.Error()).To(Equal("Authentication failed: 401 unauthorized"))
			})

			It("should return message only when no wrapped error", func() {
				err := aianalysis.NewPermanentError("Configuration invalid", "InvalidConfig", nil)

				Expect(err.Error()).To(Equal("Configuration invalid"))
			})
		})

		Context("Unwrap() method", func() {
			It("should return wrapped error for error chain inspection", func() {
				wrappedErr := errors.New("404 not found")
				err := aianalysis.NewPermanentError("Workflow not found", "NotFound", wrappedErr)

				Expect(errors.Unwrap(err)).To(Equal(wrappedErr))
			})

			It("should return nil when no wrapped error", func() {
				err := aianalysis.NewPermanentError("Invalid input", "ValidationFailed", nil)

				Expect(errors.Unwrap(err)).To(BeNil())
			})
		})

		Context("Constructor", func() {
			It("should create permanent error with reason for debugging", func() {
				wrappedErr := errors.New("forbidden")
				err := aianalysis.NewPermanentError("Access denied", "Forbidden", wrappedErr)

				Expect(err).NotTo(BeNil())
				Expect(err.Message).To(Equal("Access denied"))
				Expect(err.Reason).To(Equal("Forbidden"))
				Expect(err.Err).To(Equal(wrappedErr))
			})
		})
	})

	// ========================================
	// VALIDATION ERROR
	// ========================================
	Describe("ValidationError", func() {
		// BR-AI-021: Validation errors indicate user input problems
		// Business Value: Clear feedback for CRD spec corrections

		Context("Error() method", func() {
			It("should include field name and message", func() {
				err := aianalysis.NewValidationError("signalID", "cannot be empty")

				Expect(err.Error()).To(Equal("validation error for signalID: cannot be empty"))
			})
		})

		Context("Constructor", func() {
			It("should create validation error with field context", func() {
				err := aianalysis.NewValidationError("confidence", "must be between 0 and 1")

				Expect(err).NotTo(BeNil())
				Expect(err.Field).To(Equal("confidence"))
				Expect(err.Message).To(Equal("must be between 0 and 1"))
			})
		})
	})

	// ========================================
	// ERROR TYPE IDENTIFICATION
	// ========================================
	Describe("Error Type Identification", func() {
		// Business Value: Handlers can route errors appropriately

		It("should identify transient errors using errors.As", func() {
			err := aianalysis.NewTransientError("timeout", nil)
			var transientErr *aianalysis.TransientError
			Expect(errors.As(err, &transientErr)).To(BeTrue())
		})

		It("should identify permanent errors using errors.As", func() {
			err := aianalysis.NewPermanentError("forbidden", "AccessDenied", nil)
			var permanentErr *aianalysis.PermanentError
			Expect(errors.As(err, &permanentErr)).To(BeTrue())
		})

		It("should identify validation errors using errors.As", func() {
			err := aianalysis.NewValidationError("field", "invalid")
			var validationErr *aianalysis.ValidationError
			Expect(errors.As(err, &validationErr)).To(BeTrue())
		})

		It("should NOT misidentify error types", func() {
			transientErr := aianalysis.NewTransientError("timeout", nil)
			var permanentErr *aianalysis.PermanentError
			Expect(errors.As(transientErr, &permanentErr)).To(BeFalse())
		})
	})
})


