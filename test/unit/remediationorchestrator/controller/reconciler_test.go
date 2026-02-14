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

package controller

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func TestController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

// BR-ORCH-025: Core Orchestration Workflow
var _ = Describe("BR-ORCH-025: RemediationOrchestrator Controller", func() {
	var (
		ctx        context.Context
		scheme     *runtime.Scheme
		reconciler *prodcontroller.Reconciler
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Build scheme with all required types
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = signalprocessingv1.AddToScheme(scheme)
		_ = aianalysisv1.AddToScheme(scheme)
		_ = workflowexecutionv1.AddToScheme(scheme)
		_ = notificationv1.AddToScheme(scheme)
		_ = eav1.AddToScheme(scheme)

		// Create fake client and reconciler
		// Audit store is nil for unit tests (DD-AUDIT-003 compliant - audit is optional)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		timeoutConfig := prodcontroller.TimeoutConfig{}                                                                                                                                                                       // Use default timeout config
		recorder := record.NewFakeRecorder(20)                                                                                                                                                                                 // DD-EVENT-001: FakeRecorder for K8s event assertions
		reconciler = prodcontroller.NewReconciler(fakeClient, fakeClient, scheme, nil, recorder, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()), timeoutConfig, nil) // Use default routing (will be created)
	})

	Describe("NewReconciler", func() {
		It("should return a non-nil reconciler to enable BR-ORCH-025 orchestration", func() {
			Expect(reconciler).ToNot(BeNil())
		})
	})

	Describe("Reconcile", func() {
		It("should return without error for non-existent RemediationRequest", func() {
			// Non-existent RR returns no error (object not found is graceful)
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent-rr",
					Namespace: "default",
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	// Note: Additional tests for phase transitions and handler integration
	// are in test/unit/remediationorchestrator/ following TDD patterns
})
