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

// Package workflowexecution contains the WorkflowExecution controller
// This test suite follows TDD methodology - tests written FIRST
// Uses fake client for unit tests (no envtest dependency)
package workflowexecution

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Package-level test variables
var (
	ctx        context.Context
	cancel     context.CancelFunc
	testScheme *runtime.Scheme
	k8sClient  client.Client
)

func TestWorkflowExecution(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "WorkflowExecution Controller Suite - TDD")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	By("setting up test scheme")
	testScheme = runtime.NewScheme()
	Expect(scheme.AddToScheme(testScheme)).To(Succeed())
	Expect(workflowexecutionv1.AddToScheme(testScheme)).To(Succeed())
	Expect(tektonv1.AddToScheme(testScheme)).To(Succeed())

	By("creating fake k8s client")
	k8sClient = fake.NewClientBuilder().
		WithScheme(testScheme).
		WithStatusSubresource(&workflowexecutionv1.WorkflowExecution{}).
		Build()
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test suite")
})

// =============================================================================
// Test Helper Functions
// =============================================================================

// newTestWorkflowExecution creates a test WorkflowExecution with required fields
func newTestWorkflowExecution(name, namespace string) *workflowexecutionv1.WorkflowExecution {
	return &workflowexecutionv1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: workflowexecutionv1.WorkflowExecutionSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name:      "test-remediation-request",
				Namespace: namespace,
			},
			WorkflowRef: workflowexecutionv1.WorkflowRef{
				WorkflowID:     "increase-memory-conservative",
				Version:        "v1.0.0",
				ContainerImage: "ghcr.io/kubernaut/workflows/increase-memory:v1.0.0",
			},
			TargetResource: namespace + "/deployment/test-app",
			Parameters: map[string]string{
				"MEMORY_INCREMENT_MB": "256",
				"NAMESPACE":           namespace,
			},
		},
	}
}

// newTestWorkflowExecutionWithTarget creates a test WFE with specific target
func newTestWorkflowExecutionWithTarget(name, namespace, target string) *workflowexecutionv1.WorkflowExecution {
	wfe := newTestWorkflowExecution(name, namespace)
	wfe.Spec.TargetResource = target
	return wfe
}

// newTestWorkflowExecutionWithWorkflow creates a test WFE with specific workflow
func newTestWorkflowExecutionWithWorkflow(name, namespace, workflowID string) *workflowexecutionv1.WorkflowExecution {
	wfe := newTestWorkflowExecution(name, namespace)
	wfe.Spec.WorkflowRef.WorkflowID = workflowID
	return wfe
}

// createTestNamespace creates a test namespace (no-op for fake client)
func createTestNamespace(name string) *corev1.Namespace {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	// Fake client doesn't need namespace created
	return ns
}

// waitForPhase waits for WFE to reach expected phase
func waitForPhase(name, namespace, expectedPhase string, timeout time.Duration) {
	Eventually(func() string {
		var wfe workflowexecutionv1.WorkflowExecution
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &wfe)
		if err != nil {
			return ""
		}
		return wfe.Status.Phase
	}, timeout, 100*time.Millisecond).Should(Equal(expectedPhase))
}

// newFakeClient creates a fresh fake client for isolated tests
func newFakeClient(objects ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(testScheme).
		WithStatusSubresource(&workflowexecutionv1.WorkflowExecution{}).
		WithObjects(objects...).
		Build()
}
