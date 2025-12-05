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

package remediationorchestrator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/controller"
)

var _ = Describe("Controller (BR-ORCH-025, BR-ORCH-026)", func() {

	Describe("Reconciler", func() {
		var reconciler *controller.Reconciler

		BeforeEach(func() {
			reconciler = controller.NewReconciler()
		})

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
