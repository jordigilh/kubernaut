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

package authwebhook

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/test/testutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// #204 / Change 0b: CRD structural-schema format hardening. These IT tests go
// straight through the real envtest API server via k8sClient.Create (NOT
// through rwHandler/atHandler.Handle(), which is a direct in-process call that
// never touches the apiserver's OpenAPI schema validation). Kubebuilder
// `Pattern` markers are enforced by the apiserver itself, before any admission
// webhook is even invoked, so this is the only way to prove the constraint.
//
// Business Requirements: BR-WORKFLOW-006, BR-WORKFLOW-007.

var _ = Describe("IT-CRD-312 CRD Schema Format Hardening", Label("integration", "authwebhook", "crd-schema"), func() {

	crdUniqueID := func(prefix string) string {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}

	// validRW returns a RemediationWorkflow that satisfies every existing
	// required/MinItems constraint, so a single field can be mutated to
	// isolate exactly one Pattern violation per test case.
	validRW := func(name string) *rwv1alpha1.RemediationWorkflow {
		return &rwv1alpha1.RemediationWorkflow{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "kubernaut.ai/v1alpha1",
				Kind:       "RemediationWorkflow",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: rwv1alpha1.RemediationWorkflowSpec{
				Version:    "1.0.0",
				ActionType: "ScaleReplicas",
				Description: rwv1alpha1.RemediationWorkflowDescription{
					What:      "IT-CRD-312 CRD schema format hardening test workflow",
					WhenToUse: "For CRD Pattern validation integration testing",
				},
				Labels: rwv1alpha1.RemediationWorkflowLabels{
					Severity:    []string{"critical"},
					Environment: []string{"production"},
					Component:   []string{"v1/Pod"},
					Priority:    "P1",
				},
				Execution: rwv1alpha1.RemediationWorkflowExecution{
					Engine: "job",
					Bundle: testutil.ValidBundleRef,
				},
				Parameters: []rwv1alpha1.RemediationWorkflowParameter{
					{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
				},
			},
		}
	}

	validAT := func(name string) *atv1alpha1.ActionType {
		return &atv1alpha1.ActionType{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "kubernaut.ai/v1alpha1",
				Kind:       "ActionType",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: atv1alpha1.ActionTypeSpec{
				Name: "ScaleReplicas",
				Description: atv1alpha1.ActionTypeDescription{
					What:      "IT-CRD-312 CRD schema format hardening test action type",
					WhenToUse: "For CRD Pattern validation integration testing",
				},
			},
		}
	}

	Context("RemediationWorkflow.spec.version (semver Pattern)", func() {
		It("IT-CRD-312-001a: rejects a non-semver version with apierrors.IsInvalid", func() {
			rw := validRW(crdUniqueID("it-crd-badversion"))
			rw.Spec.Version = "v1.0" // missing patch segment, leading "v" not allowed

			err := k8sClient.Create(ctx, rw)
			Expect(err).To(HaveOccurred(), "apiserver should reject a non-semver version")
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "rejection should be a structural schema Invalid error, got: %v", err)
		})

		It("IT-CRD-312-001b: accepts a well-formed semver version", func() {
			rw := validRW(crdUniqueID("it-crd-goodversion"))
			rw.Spec.Version = "1.2.3"

			err := k8sClient.Create(ctx, rw)
			Expect(err).ToNot(HaveOccurred(), "apiserver should accept a well-formed semver version: %v", err)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rw) })
		})
	})

	Context("RemediationWorkflow.spec.actionType (PascalCase Pattern)", func() {
		It("IT-CRD-312-002a: rejects a non-PascalCase actionType with apierrors.IsInvalid", func() {
			rw := validRW(crdUniqueID("it-crd-badactiontype"))
			rw.Spec.ActionType = "scale_replicas" // snake_case, not PascalCase

			err := k8sClient.Create(ctx, rw)
			Expect(err).To(HaveOccurred(), "apiserver should reject a non-PascalCase actionType")
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "rejection should be a structural schema Invalid error, got: %v", err)
		})

		It("IT-CRD-312-002b: accepts a well-formed PascalCase actionType", func() {
			rw := validRW(crdUniqueID("it-crd-goodactiontype"))
			rw.Spec.ActionType = "RestartPod"

			err := k8sClient.Create(ctx, rw)
			Expect(err).ToNot(HaveOccurred(), "apiserver should accept a well-formed PascalCase actionType: %v", err)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rw) })
		})
	})

	Context("RemediationWorkflow.spec.maintainers[].email (email Pattern)", func() {
		It("IT-CRD-312-003a: rejects a malformed maintainer email with apierrors.IsInvalid", func() {
			rw := validRW(crdUniqueID("it-crd-bademail"))
			rw.Spec.Maintainers = []rwv1alpha1.RemediationWorkflowMaintainer{
				{Name: "Test Maintainer", Email: "not-an-email"},
			}

			err := k8sClient.Create(ctx, rw)
			Expect(err).To(HaveOccurred(), "apiserver should reject a malformed maintainer email")
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "rejection should be a structural schema Invalid error, got: %v", err)
		})

		It("IT-CRD-312-003b: accepts a well-formed maintainer email", func() {
			rw := validRW(crdUniqueID("it-crd-goodemail"))
			rw.Spec.Maintainers = []rwv1alpha1.RemediationWorkflowMaintainer{
				{Name: "Test Maintainer", Email: "maintainer@example.com"},
			}

			err := k8sClient.Create(ctx, rw)
			Expect(err).ToNot(HaveOccurred(), "apiserver should accept a well-formed maintainer email: %v", err)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rw) })
		})
	})

	Context("ActionType.spec.name (PascalCase Pattern)", func() {
		It("IT-CRD-312-004a: rejects a non-PascalCase action type name with apierrors.IsInvalid", func() {
			at := validAT(crdUniqueID("it-crd-badatname"))
			at.Spec.Name = "restart-pod" // kebab-case, not PascalCase

			err := k8sClient.Create(ctx, at)
			Expect(err).To(HaveOccurred(), "apiserver should reject a non-PascalCase ActionType name")
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "rejection should be a structural schema Invalid error, got: %v", err)
		})

		It("IT-CRD-312-004b: accepts a well-formed PascalCase action type name", func() {
			at := validAT(crdUniqueID("it-crd-goodatname"))
			at.Spec.Name = "RestartPod"

			err := k8sClient.Create(ctx, at)
			Expect(err).ToNot(HaveOccurred(), "apiserver should accept a well-formed PascalCase ActionType name: %v", err)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, at) })
		})
	})
})
