package datastorage

import (
	"encoding/json"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

// Test entry point moved to notification_audit_validator_test.go to avoid "Rerunning Suite" error

var _ = Describe("ValidationError", func() {
	var validationErr *validation.ValidationError

	BeforeEach(func() {
		validationErr = validation.NewValidationError("notification_audit", "validation failed")
	})

	Context("Error Creation", func() {
		It("should create a validation error with resource and message", func() {
			Expect(validationErr.Resource).To(Equal("notification_audit"))
			Expect(validationErr.Message).To(Equal("validation failed"))
			Expect(validationErr.FieldErrors).ToNot(BeNil())
			Expect(len(validationErr.FieldErrors)).To(Equal(0))
		})
	})

	Context("Field Errors", func() {
		It("should add field errors", func() {
			validationErr.AddFieldError("field1", "error1")
			validationErr.AddFieldError("field2", "error2")

			Expect(len(validationErr.FieldErrors)).To(Equal(2))
			Expect(validationErr.FieldErrors["field1"]).To(Equal("error1"))
			Expect(validationErr.FieldErrors["field2"]).To(Equal("error2"))
		})

		It("should overwrite existing field error", func() {
			validationErr.AddFieldError("field1", "error1")
			validationErr.AddFieldError("field1", "error2")

			Expect(len(validationErr.FieldErrors)).To(Equal(1))
			Expect(validationErr.FieldErrors["field1"]).To(Equal("error2"))
		})
	})

	Context("Error Interface", func() {
		It("should return error string without field errors", func() {
			errStr := validationErr.Error()
			Expect(errStr).To(ContainSubstring("notification_audit"))
			Expect(errStr).To(ContainSubstring("validation failed"))
		})

		It("should return error string with field errors", func() {
			validationErr.AddFieldError("field1", "error1")
			errStr := validationErr.Error()
			Expect(errStr).To(ContainSubstring("notification_audit"))
			Expect(errStr).To(ContainSubstring("validation failed"))
			Expect(errStr).To(ContainSubstring("fields"))
		})
	})

	Context("RFC 7807 Conversion", func() {
		It("should convert to RFC 7807 problem", func() {
			validationErr.AddFieldError("field1", "error1")
			validationErr.AddFieldError("field2", "error2")

			problem := validationErr.ToRFC7807()

			Expect(problem.Type).To(Equal("https://kubernaut.io/errors/validation-error"))
			Expect(problem.Title).To(Equal("Validation Error"))
			Expect(problem.Status).To(Equal(http.StatusBadRequest))
			Expect(problem.Detail).To(Equal("validation failed"))
			Expect(problem.Instance).To(Equal("/audit/notification_audit"))
			Expect(problem.Extensions["resource"]).To(Equal("notification_audit"))
			Expect(problem.Extensions["field_errors"]).To(Equal(validationErr.FieldErrors))
		})
	})
})

var _ = Describe("validation.RFC7807Problem", func() {
	Context("Validation Error Problem", func() {
		It("should create validation error problem", func() {
			fieldErrors := map[string]string{
				"field1": "error1",
				"field2": "error2",
			}
			problem := validation.NewValidationErrorProblem("notification_audit", fieldErrors)

			Expect(problem.Type).To(Equal("https://kubernaut.io/errors/validation-error"))
			Expect(problem.Title).To(Equal("Validation Error"))
			Expect(problem.Status).To(Equal(http.StatusBadRequest))
			Expect(problem.Detail).To(ContainSubstring("notification_audit"))
			Expect(problem.Instance).To(Equal("/audit/notification_audit"))
			Expect(problem.Extensions["resource"]).To(Equal("notification_audit"))
			Expect(problem.Extensions["field_errors"]).To(Equal(fieldErrors))
		})
	})

	Context("Not Found Problem", func() {
		It("should create not found problem", func() {
			problem := validation.NewNotFoundProblem("notification_audit", "test-id-123")

			Expect(problem.Type).To(Equal("https://kubernaut.io/errors/not-found"))
			Expect(problem.Title).To(Equal("Resource Not Found"))
			Expect(problem.Status).To(Equal(http.StatusNotFound))
			Expect(problem.Detail).To(ContainSubstring("test-id-123"))
			Expect(problem.Instance).To(Equal("/audit/notification_audit/test-id-123"))
			Expect(problem.Extensions["resource"]).To(Equal("notification_audit"))
			Expect(problem.Extensions["id"]).To(Equal("test-id-123"))
		})
	})

	Context("Internal Error Problem", func() {
		It("should create internal error problem", func() {
			problem := validation.NewInternalErrorProblem("database connection failed")

			Expect(problem.Type).To(Equal("https://kubernaut.io/errors/internal-error"))
			Expect(problem.Title).To(Equal("Internal Server Error"))
			Expect(problem.Status).To(Equal(http.StatusInternalServerError))
			Expect(problem.Detail).To(Equal("database connection failed"))
			Expect(problem.Extensions["retry"]).To(BeTrue())
		})
	})

	Context("Service Unavailable Problem", func() {
		It("should create service unavailable problem", func() {
			problem := validation.NewServiceUnavailableProblem("database is down")

			Expect(problem.Type).To(Equal("https://kubernaut.io/errors/service-unavailable"))
			Expect(problem.Title).To(Equal("Service Unavailable"))
			Expect(problem.Status).To(Equal(http.StatusServiceUnavailable))
			Expect(problem.Detail).To(Equal("database is down"))
			Expect(problem.Extensions["retry"]).To(BeTrue())
		})
	})

	Context("Conflict Problem", func() {
		It("should create conflict problem", func() {
			problem := validation.NewConflictProblem("notification_audit", "notification_id", "test-id-123")

			Expect(problem.Type).To(Equal("https://kubernaut.io/errors/conflict"))
			Expect(problem.Title).To(Equal("Resource Conflict"))
			Expect(problem.Status).To(Equal(http.StatusConflict))
			Expect(problem.Detail).To(ContainSubstring("test-id-123"))
			Expect(problem.Instance).To(Equal("/audit/notification_audit"))
			Expect(problem.Extensions["resource"]).To(Equal("notification_audit"))
			Expect(problem.Extensions["field"]).To(Equal("notification_id"))
			Expect(problem.Extensions["value"]).To(Equal("test-id-123"))
		})
	})

	Context("JSON Marshaling", func() {
		It("should marshal to RFC 7807 compliant JSON", func() {
			problem := &validation.RFC7807Problem{
				Type:     "https://kubernaut.io/errors/validation-error",
				Title:    "Validation Error",
				Status:   http.StatusBadRequest,
				Detail:   "validation failed",
				Instance: "/audit/notification_audit",
				Extensions: map[string]interface{}{
					"resource": "notification_audit",
					"field_errors": map[string]string{
						"field1": "error1",
					},
				},
			}

		jsonBytes, err := json.Marshal(problem)
		Expect(err).ToNot(HaveOccurred())

		var result validation.RFC7807Problem
		err = json.Unmarshal(jsonBytes, &result)
		Expect(err).ToNot(HaveOccurred())

		// Verify standard RFC 7807 fields (type-safe access)
		Expect(result.Type).To(Equal("https://kubernaut.io/errors/validation-error"))
		Expect(result.Title).To(Equal("Validation Error"))
		Expect(result.Status).To(Equal(400))
		Expect(result.Detail).To(Equal("validation failed"))
		Expect(result.Instance).To(Equal("/audit/notification_audit"))

		// Verify extensions are captured correctly
		Expect(result.Extensions["resource"]).To(Equal("notification_audit"))
		Expect(result.Extensions["field_errors"]).ToNot(BeNil())
		})

		It("should omit optional fields when empty", func() {
			problem := &validation.RFC7807Problem{
				Type:   "https://kubernaut.io/errors/internal-error",
				Title:  "Internal Server Error",
				Status: http.StatusInternalServerError,
			}

		jsonBytes, err := json.Marshal(problem)
		Expect(err).ToNot(HaveOccurred())

		var result validation.RFC7807Problem
		err = json.Unmarshal(jsonBytes, &result)
		Expect(err).ToNot(HaveOccurred())

		Expect(result.Type).To(Equal("https://kubernaut.io/errors/internal-error"))
		Expect(result.Title).To(Equal("Internal Server Error"))
		Expect(result.Status).To(Equal(500))
		Expect(result.Detail).To(BeEmpty(), "Optional detail field should be empty")
		Expect(result.Instance).To(BeEmpty(), "Optional instance field should be empty")
		})
	})

	Context("Error Interface", func() {
		It("should return error string", func() {
			problem := &validation.RFC7807Problem{
				Type:   "https://kubernaut.io/errors/validation-error",
				Title:  "Validation Error",
				Status: http.StatusBadRequest,
				Detail: "validation failed",
			}

			errStr := problem.Error()
			Expect(errStr).To(ContainSubstring("Validation Error"))
			Expect(errStr).To(ContainSubstring("validation failed"))
			Expect(errStr).To(ContainSubstring("400"))
		})
	})
})

