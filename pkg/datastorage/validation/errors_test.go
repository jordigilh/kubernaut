package validation

import (
	"encoding/json"
	"net/http"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestValidationErrors(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Validation Errors Suite")
}

var _ = Describe("ValidationError", func() {
	var validationErr *ValidationError

	BeforeEach(func() {
		validationErr = NewValidationError("notification_audit", "validation failed")
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

var _ = Describe("RFC7807Problem", func() {
	Context("Validation Error Problem", func() {
		It("should create validation error problem", func() {
			fieldErrors := map[string]string{
				"field1": "error1",
				"field2": "error2",
			}
			problem := NewValidationErrorProblem("notification_audit", fieldErrors)

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
			problem := NewNotFoundProblem("notification_audit", "test-id-123")

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
			problem := NewInternalErrorProblem("database connection failed")

			Expect(problem.Type).To(Equal("https://kubernaut.io/errors/internal-error"))
			Expect(problem.Title).To(Equal("Internal Server Error"))
			Expect(problem.Status).To(Equal(http.StatusInternalServerError))
			Expect(problem.Detail).To(Equal("database connection failed"))
			Expect(problem.Extensions["retry"]).To(BeTrue())
		})
	})

	Context("Service Unavailable Problem", func() {
		It("should create service unavailable problem", func() {
			problem := NewServiceUnavailableProblem("database is down")

			Expect(problem.Type).To(Equal("https://kubernaut.io/errors/service-unavailable"))
			Expect(problem.Title).To(Equal("Service Unavailable"))
			Expect(problem.Status).To(Equal(http.StatusServiceUnavailable))
			Expect(problem.Detail).To(Equal("database is down"))
			Expect(problem.Extensions["retry"]).To(BeTrue())
		})
	})

	Context("Conflict Problem", func() {
		It("should create conflict problem", func() {
			problem := NewConflictProblem("notification_audit", "notification_id", "test-id-123")

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
			problem := &RFC7807Problem{
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

			var result map[string]interface{}
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			// Verify standard RFC 7807 fields
			Expect(result["type"]).To(Equal("https://kubernaut.io/errors/validation-error"))
			Expect(result["title"]).To(Equal("Validation Error"))
			Expect(result["status"]).To(BeNumerically("==", 400))
			Expect(result["detail"]).To(Equal("validation failed"))
			Expect(result["instance"]).To(Equal("/audit/notification_audit"))

			// Verify extensions are flattened into top-level JSON
			Expect(result["resource"]).To(Equal("notification_audit"))
			Expect(result["field_errors"]).ToNot(BeNil())
		})

		It("should omit optional fields when empty", func() {
			problem := &RFC7807Problem{
				Type:   "https://kubernaut.io/errors/internal-error",
				Title:  "Internal Server Error",
				Status: http.StatusInternalServerError,
			}

			jsonBytes, err := json.Marshal(problem)
			Expect(err).ToNot(HaveOccurred())

			var result map[string]interface{}
			err = json.Unmarshal(jsonBytes, &result)
			Expect(err).ToNot(HaveOccurred())

			Expect(result["type"]).To(Equal("https://kubernaut.io/errors/internal-error"))
			Expect(result["title"]).To(Equal("Internal Server Error"))
			Expect(result["status"]).To(BeNumerically("==", 500))
			Expect(result).ToNot(HaveKey("detail"))
			Expect(result).ToNot(HaveKey("instance"))
		})
	})

	Context("Error Interface", func() {
		It("should return error string", func() {
			problem := &RFC7807Problem{
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

