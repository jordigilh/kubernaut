package errors

import (
	"errors"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Structured Errors", func() {
	Describe("AppError", func() {
		Context("basic error creation", func() {
			It("should create error with correct properties", func() {
				err := New(ErrorTypeValidation, "test message")
				
				Expect(err.Type).To(Equal(ErrorTypeValidation))
				Expect(err.Message).To(Equal("test message"))
				Expect(err.StatusCode).To(Equal(http.StatusBadRequest))
				Expect(err.Details).To(BeEmpty())
				Expect(err.Cause).To(BeNil())
			})

			It("should implement error interface correctly", func() {
				err := New(ErrorTypeValidation, "test message")
				
				Expect(err.Error()).To(Equal("validation: test message"))
			})

			It("should include details in error string when present", func() {
				err := New(ErrorTypeValidation, "test message").WithDetails("extra info")
				
				Expect(err.Error()).To(Equal("validation: test message (extra info)"))
			})
		})

		Context("error wrapping", func() {
			It("should wrap underlying error", func() {
				originalErr := errors.New("original error")
				wrappedErr := Wrap(originalErr, ErrorTypeDatabase, "operation failed")
				
				Expect(wrappedErr.Type).To(Equal(ErrorTypeDatabase))
				Expect(wrappedErr.Message).To(Equal("operation failed"))
				Expect(wrappedErr.Cause).To(Equal(originalErr))
				Expect(wrappedErr.Unwrap()).To(Equal(originalErr))
			})

			It("should format wrapped error with arguments", func() {
				originalErr := errors.New("connection refused")
				wrappedErr := Wrapf(originalErr, ErrorTypeNetwork, "failed to connect to %s:%d", "localhost", 5432)
				
				Expect(wrappedErr.Message).To(Equal("failed to connect to localhost:5432"))
				Expect(wrappedErr.Cause).To(Equal(originalErr))
			})
		})

		Context("adding details", func() {
			It("should add details to existing error", func() {
				err := New(ErrorTypeAuth, "authentication failed")
				detailedErr := err.WithDetails("invalid token")
				
				Expect(detailedErr.Details).To(Equal("invalid token"))
				Expect(detailedErr).To(BeIdenticalTo(err)) // Should modify in place
			})

			It("should add formatted details", func() {
				err := New(ErrorTypeAuth, "authentication failed")
				detailedErr := err.WithDetailsf("user %s, attempt %d", "john", 3)
				
				Expect(detailedErr.Details).To(Equal("user john, attempt 3"))
			})
		})
	})

	Describe("HTTP Status Code Mapping", func() {
		It("should map error types to correct HTTP status codes", func() {
			testCases := []struct {
				errorType  ErrorType
				statusCode int
			}{
				{ErrorTypeValidation, http.StatusBadRequest},
				{ErrorTypeAuth, http.StatusUnauthorized},
				{ErrorTypeNotFound, http.StatusNotFound},
				{ErrorTypeConflict, http.StatusConflict},
				{ErrorTypeTimeout, http.StatusRequestTimeout},
				{ErrorTypeRateLimit, http.StatusTooManyRequests},
				{ErrorTypeDatabase, http.StatusInternalServerError},
				{ErrorTypeNetwork, http.StatusInternalServerError},
				{ErrorTypeInternal, http.StatusInternalServerError},
			}

			for _, tc := range testCases {
				err := New(tc.errorType, "test message")
				Expect(err.StatusCode).To(Equal(tc.statusCode))
			}
		})
	})

	Describe("Predefined Error Constructors", func() {
		It("should create validation error", func() {
			err := NewValidationError("invalid input")
			
			Expect(err.Type).To(Equal(ErrorTypeValidation))
			Expect(err.Message).To(Equal("invalid input"))
		})

		It("should create database error", func() {
			originalErr := errors.New("connection lost")
			err := NewDatabaseError("query", originalErr)
			
			Expect(err.Type).To(Equal(ErrorTypeDatabase))
			Expect(err.Message).To(ContainSubstring("database operation failed: query"))
			Expect(err.Cause).To(Equal(originalErr))
		})

		It("should create not found error", func() {
			err := NewNotFoundError("user")
			
			Expect(err.Type).To(Equal(ErrorTypeNotFound))
			Expect(err.Message).To(Equal("user not found"))
		})

		It("should create auth error", func() {
			err := NewAuthError("invalid credentials")
			
			Expect(err.Type).To(Equal(ErrorTypeAuth))
			Expect(err.Message).To(Equal("invalid credentials"))
		})

		It("should create timeout error", func() {
			err := NewTimeoutError("database query")
			
			Expect(err.Type).To(Equal(ErrorTypeTimeout))
			Expect(err.Message).To(Equal("operation timed out: database query"))
		})
	})

	Describe("Error Type Checking", func() {
		It("should correctly identify error types", func() {
			validationErr := NewValidationError("test")
			authErr := NewAuthError("test")
			
			Expect(IsType(validationErr, ErrorTypeValidation)).To(BeTrue())
			Expect(IsType(validationErr, ErrorTypeAuth)).To(BeFalse())
			Expect(IsType(authErr, ErrorTypeAuth)).To(BeTrue())
		})

		It("should handle non-AppError types", func() {
			regularErr := errors.New("regular error")
			
			Expect(IsType(regularErr, ErrorTypeValidation)).To(BeFalse())
			Expect(GetType(regularErr)).To(Equal(ErrorTypeInternal))
		})

		It("should get correct status codes", func() {
			validationErr := NewValidationError("test")
			regularErr := errors.New("regular error")
			
			Expect(GetStatusCode(validationErr)).To(Equal(http.StatusBadRequest))
			Expect(GetStatusCode(regularErr)).To(Equal(http.StatusInternalServerError))
		})
	})

	Describe("Safe Error Messages", func() {
		It("should return safe messages for different error types", func() {
			testCases := []struct {
				errorType     ErrorType
				expectedSafe  string
			}{
				{ErrorTypeValidation, ""},     // Validation messages are passed through
				{ErrorTypeNotFound, ErrorMessages.ResourceNotFound},
				{ErrorTypeAuth, ErrorMessages.AuthenticationFailed},
				{ErrorTypeTimeout, ErrorMessages.OperationTimeout},
				{ErrorTypeRateLimit, ErrorMessages.RateLimitExceeded},
				{ErrorTypeConflict, ErrorMessages.ConcurrentModification},
				{ErrorTypeDatabase, "An internal error occurred"},
			}

			for _, tc := range testCases {
				var err error
				switch tc.errorType {
				case ErrorTypeValidation:
					err = NewValidationError("specific validation message")
					Expect(SafeErrorMessage(err)).To(Equal("specific validation message"))
					continue
				default:
					err = New(tc.errorType, "internal details")
				}
				
				Expect(SafeErrorMessage(err)).To(Equal(tc.expectedSafe))
			}
		})

		It("should return generic message for regular errors", func() {
			regularErr := errors.New("internal panic")
			safeMsg := SafeErrorMessage(regularErr)
			
			Expect(safeMsg).To(Equal("An unexpected error occurred"))
		})
	})

	Describe("Logging Fields", func() {
		It("should generate structured logging fields", func() {
			originalErr := errors.New("connection failed")
			appErr := Wrapf(originalErr, ErrorTypeDatabase, "query failed").
				WithDetails("table: users")
			
			fields := LogFields(appErr)
			
			Expect(fields).To(HaveKey("error"))
			Expect(fields).To(HaveKey("error_type"))
			Expect(fields).To(HaveKey("status_code"))
			Expect(fields).To(HaveKey("error_details"))
			Expect(fields).To(HaveKey("underlying_error"))
			
			Expect(fields["error_type"]).To(Equal("database"))
			Expect(fields["status_code"]).To(Equal(http.StatusInternalServerError))
			Expect(fields["error_details"]).To(Equal("table: users"))
			Expect(fields["underlying_error"]).To(Equal("connection failed"))
		})

		It("should handle simple AppError without details", func() {
			err := NewValidationError("invalid input")
			fields := LogFields(err)
			
			Expect(fields).To(HaveKey("error"))
			Expect(fields).To(HaveKey("error_type"))
			Expect(fields).To(HaveKey("status_code"))
			Expect(fields).NotTo(HaveKey("error_details"))
			Expect(fields).NotTo(HaveKey("underlying_error"))
		})

		It("should handle regular errors", func() {
			err := errors.New("regular error")
			fields := LogFields(err)
			
			Expect(fields).To(HaveKey("error"))
			Expect(fields).NotTo(HaveKey("error_type"))
		})
	})

	Describe("Error Chaining", func() {
		It("should handle empty error list", func() {
			err := Chain()
			Expect(err).To(BeNil())
		})

		It("should handle single error", func() {
			originalErr := errors.New("single error")
			err := Chain(originalErr)
			
			Expect(err).To(Equal(originalErr))
		})

		It("should filter nil errors", func() {
			err1 := errors.New("error 1")
			err2 := errors.New("error 2")
			
			err := Chain(err1, nil, err2, nil)
			
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error 1"))
			Expect(err.Error()).To(ContainSubstring("error 2"))
		})

		It("should chain multiple errors", func() {
			err1 := errors.New("first error")
			err2 := errors.New("second error")
			err3 := errors.New("third error")
			
			chainedErr := Chain(err1, err2, err3)
			
			Expect(chainedErr).To(HaveOccurred())
			errMsg := chainedErr.Error()
			Expect(errMsg).To(ContainSubstring("first error"))
			Expect(errMsg).To(ContainSubstring("second error"))
			Expect(errMsg).To(ContainSubstring("third error"))
			Expect(errMsg).To(ContainSubstring(" -> "))
		})

		It("should return nil when all errors are nil", func() {
			err := Chain(nil, nil, nil)
			Expect(err).To(BeNil())
		})
	})

	Describe("Error Type Constants", func() {
		It("should have all expected error types defined", func() {
			expectedTypes := []ErrorType{
				ErrorTypeValidation,
				ErrorTypeDatabase,
				ErrorTypeNetwork,
				ErrorTypeAuth,
				ErrorTypeNotFound,
				ErrorTypeConflict,
				ErrorTypeInternal,
				ErrorTypeTimeout,
				ErrorTypeRateLimit,
			}

			for _, errorType := range expectedTypes {
				Expect(string(errorType)).NotTo(BeEmpty())
			}
		})
	})
})