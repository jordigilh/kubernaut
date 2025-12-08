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

package remediationorchestrator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/controller"
)

var _ = Describe("Controller (BR-ORCH-025, BR-ORCH-026)", func() {
	var (
		scheme     *runtime.Scheme
		reconciler *controller.Reconciler
	)

	BeforeEach(func() {
		// Build scheme with all required types
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = signalprocessingv1.AddToScheme(scheme)
		_ = aianalysisv1.AddToScheme(scheme)
		_ = workflowexecutionv1.AddToScheme(scheme)
		_ = notificationv1.AddToScheme(scheme)

		// Create fake client and reconciler
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler = controller.NewReconciler(fakeClient, scheme)
	})

	Describe("Reconciler", func() {
		Context("when creating a new Reconciler", func() {
			It("should return a non-nil Reconciler", func() {
				Expect(reconciler).ToNot(BeNil())
			})
		})

		Context("when checking interface compliance", func() {
			It("should implement controller-runtime Reconciler interface", func() {
				// Compile-time interface satisfaction check
				var _ reconcile.Reconciler = reconciler
				Expect(reconciler).ToNot(BeNil())
			})

			It("should have SetupWithManager method for controller registration", func() {
				// Verify method exists (compile-time check)
				Expect(reconciler.SetupWithManager).ToNot(BeNil())
			})
		})
	})
})
