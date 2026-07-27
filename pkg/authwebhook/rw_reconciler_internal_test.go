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
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// ========================================
// Issue #1674 (nilnil sentinel-error refactor), Batch 2: findActionTypeKey is
// unexported, so its "not found" branch (zero test coverage before this
// change) is exercised here via an internal (white-box) test file — the
// package already has precedent for this
// (remediationapprovalrequest_handler_fuzz_test.go).
// BR-WORKFLOW-006.
// ========================================
var _ = Describe("findActionTypeKey (Issue #1674 Batch 2)", func() {
	It("UT-AW-1674-001: returns ErrActionTypeCRDNotFound when no ActionType CRD matches the name", func() {
		s := runtime.NewScheme()
		Expect(atv1alpha1.AddToScheme(s)).To(Succeed())
		fakeK8s := fake.NewClientBuilder().WithScheme(s).Build()

		r := &RemediationWorkflowReconciler{Client: fakeK8s}

		key, err := r.findActionTypeKey(context.Background(), "NoSuchAction", "kubernaut-system")

		Expect(errors.Is(err, ErrActionTypeCRDNotFound)).To(BeTrue())
		Expect(key).To(BeNil())
	})

	It("UT-AW-1674-002: returns the ObjectKey when an ActionType CRD matches spec.name", func() {
		s := runtime.NewScheme()
		Expect(atv1alpha1.AddToScheme(s)).To(Succeed())
		at := &atv1alpha1.ActionType{}
		at.SetName("scale-memory-at")
		at.SetNamespace("kubernaut-system")
		at.Spec.Name = "ScaleMemory"
		fakeK8s := fake.NewClientBuilder().WithScheme(s).WithObjects(at).Build()

		r := &RemediationWorkflowReconciler{Client: fakeK8s}

		key, err := r.findActionTypeKey(context.Background(), "ScaleMemory", "kubernaut-system")

		Expect(err).ToNot(HaveOccurred())
		Expect(key).ToNot(BeNil())
		Expect(key.Name).To(Equal("scale-memory-at"))
	})
})
