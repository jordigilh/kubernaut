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
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	prodaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
)

var _ = Describe("ErrorClassifier", func() {
	gr := schema.GroupResource{Group: "kubernaut.ai", Resource: "remediationrequests"}

	Describe("ClassifyError", func() {
		It("should classify apierrors.NewNotFound as ERR_K8S_NOT_FOUND (retryable)", func() {
			err := apierrors.NewNotFound(gr, "rr-test-001")
			result := prodaudit.ClassifyError(err)
			Expect(result.Code).To(Equal("ERR_K8S_NOT_FOUND"))
			Expect(result.RetryPossible).To(BeTrue())
		})

		It("should classify apierrors.NewAlreadyExists as ERR_K8S_ALREADY_EXISTS (not retryable)", func() {
			err := apierrors.NewAlreadyExists(gr, "rr-test-001")
			result := prodaudit.ClassifyError(err)
			Expect(result.Code).To(Equal("ERR_K8S_ALREADY_EXISTS"))
			Expect(result.RetryPossible).To(BeFalse())
		})

		It("should classify apierrors.NewConflict as ERR_K8S_CONFLICT (retryable)", func() {
			err := apierrors.NewConflict(gr, "rr-test-001", fmt.Errorf("version mismatch"))
			result := prodaudit.ClassifyError(err)
			Expect(result.Code).To(Equal("ERR_K8S_CONFLICT"))
			Expect(result.RetryPossible).To(BeTrue())
		})

		It("should classify apierrors.NewForbidden as ERR_K8S_FORBIDDEN (not retryable)", func() {
			err := apierrors.NewForbidden(gr, "rr-test-001", fmt.Errorf("RBAC denied"))
			result := prodaudit.ClassifyError(err)
			Expect(result.Code).To(Equal("ERR_K8S_FORBIDDEN"))
			Expect(result.RetryPossible).To(BeFalse())
		})

		It("should classify apierrors.NewTimeoutError as ERR_TIMEOUT_REMEDIATION (retryable)", func() {
			err := apierrors.NewTimeoutError("request timed out", 30)
			result := prodaudit.ClassifyError(err)
			Expect(result.Code).To(Equal("ERR_TIMEOUT_REMEDIATION"))
			Expect(result.RetryPossible).To(BeTrue())
		})

		It("should classify apierrors.NewInvalid as ERR_INVALID_CONFIG (not retryable)", func() {
			gk := schema.GroupKind{Group: "kubernaut.ai", Kind: "RemediationRequest"}
			err := apierrors.NewInvalid(gk, "rr-test-001", field.ErrorList{
				field.Invalid(field.NewPath("spec", "timeout"), "-5m", "must be positive"),
			})
			result := prodaudit.ClassifyError(err)
			Expect(result.Code).To(Equal("ERR_INVALID_CONFIG"))
			Expect(result.RetryPossible).To(BeFalse())
		})

		It("should classify context.DeadlineExceeded as ERR_TIMEOUT_REMEDIATION (retryable)", func() {
			result := prodaudit.ClassifyError(context.DeadlineExceeded)
			Expect(result.Code).To(Equal("ERR_TIMEOUT_REMEDIATION"))
			Expect(result.RetryPossible).To(BeTrue())
		})

		It("should classify ErrInvalidTimeoutConfig as ERR_INVALID_TIMEOUT_CONFIG (not retryable)", func() {
			err := fmt.Errorf("validation failed: %w", prodaudit.ErrInvalidTimeoutConfig)
			result := prodaudit.ClassifyError(err)
			Expect(result.Code).To(Equal("ERR_INVALID_TIMEOUT_CONFIG"))
			Expect(result.RetryPossible).To(BeFalse())
		})

		It("should classify nil error as ERR_INTERNAL_ORCHESTRATION (retryable)", func() {
			result := prodaudit.ClassifyError(nil)
			Expect(result.Code).To(Equal("ERR_INTERNAL_ORCHESTRATION"))
			Expect(result.RetryPossible).To(BeTrue())
		})

		It("should classify unknown errors as ERR_INTERNAL_ORCHESTRATION (retryable)", func() {
			err := fmt.Errorf("something weird happened")
			result := prodaudit.ClassifyError(err)
			Expect(result.Code).To(Equal("ERR_INTERNAL_ORCHESTRATION"))
			Expect(result.RetryPossible).To(BeTrue())
		})

		It("should classify wrapped K8s errors correctly", func() {
			innerErr := apierrors.NewNotFound(gr, "rr-test-001")
			wrappedErr := fmt.Errorf("wrap: %w", innerErr)
			result := prodaudit.ClassifyError(wrappedErr)
			Expect(result.Code).To(Equal("ERR_K8S_NOT_FOUND"))
			Expect(result.RetryPossible).To(BeTrue())
		})

		It("should classify apierrors.IsServiceUnavailable as ERR_K8S_SERVICE_UNAVAILABLE (retryable)", func() {
			err := apierrors.NewServiceUnavailable("API server overloaded")
			result := prodaudit.ClassifyError(err)
			Expect(result.Code).To(Equal("ERR_K8S_SERVICE_UNAVAILABLE"))
			Expect(result.RetryPossible).To(BeTrue())
		})
	})
})
