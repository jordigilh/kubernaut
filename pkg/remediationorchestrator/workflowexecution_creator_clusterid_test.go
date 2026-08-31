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

package remediationorchestrator_test

import (
	"context"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
)

// ========================================
// RO CREATOR: workflow-declared execution cluster (Issue #2326,
// DD-FLEET-008, BR-FLEET-004)
// ========================================
// WorkflowExecution.Spec.ClusterID today is unconditionally copied from
// RemediationRequest.Spec.ClusterID (the signal's origin cluster). These
// tests pin the new precedence: the selected workflow's catalog-declared
// ExecutionClusterID (sharedtypes.WorkflowSnapshot, propagated via
// AIAnalysis.Status.RCAResult.SelectedWorkflow) wins when set; RR's
// ClusterID remains the fallback for the unchanged default case.
//
// ASVS V4.1 (Access Control Architecture) / FedRAMP AC-4 (Information Flow
// Enforcement): this introduces no new enforcement code path. Both an
// RR-supplied and a workflow-declared value land in the exact same
// WorkflowExecution.Spec.ClusterID field, which pkg/workflowexecution's
// ClientFactory.ClientFor(ctx, clusterID) resolves identically via the fleet
// MCP Gateway regardless of provenance -- proven here by asserting the
// workflow-declared value lands in that same field, not a parallel one.
// ========================================
var _ = Describe("WorkflowExecution Creator: workflow-declared execution cluster [DD-FLEET-008]", func() {
	var scheme *runtime.Scheme

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
	})

	buildRR := func(rrClusterID string) *remediationv1.RemediationRequest {
		return &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-test",
				Namespace: "kubernaut-system",
				UID:       "rr-uid-2326",
			},
			Spec: remediationv1.RemediationRequestSpec{
				ClusterID: rrClusterID,
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "nginx",
					Namespace: "default",
				},
			},
		}
	}

	buildAI := func(executionEngine, executionClusterID string) *aianalysisv1.AIAnalysis {
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aa-test",
				Namespace: "kubernaut-system",
			},
			Status: aianalysisv1.AIAnalysisStatus{
				RCAResult: &aianalysisv1.RCAResult{
					SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
						WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
							WorkflowID:         "wf-uuid-2326",
							WorkflowName:       "wf-uuid-2326",
							ActionType:         "RestartPod",
							Version:            "1.0.0",
							ExecutionBundle:    "quay.io/test:v1@sha256:abc123",
							ExecutionEngine:    executionEngine,
							ExecutionClusterID: executionClusterID,
						},
					},
				},
			},
		}
	}

	createAndGet := func(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) *workflowexecutionv1.WorkflowExecution {
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).Build()
		wec := creator.NewWorkflowExecutionCreator(k8sClient, scheme, nil)

		name, err := wec.Create(context.Background(), rr, ai)
		Expect(err).ToNot(HaveOccurred())

		created := &workflowexecutionv1.WorkflowExecution{}
		err = k8sClient.Get(context.Background(), client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
		Expect(err).ToNot(HaveOccurred())
		return created
	}

	It("UT-RO-2326-001: a workflow-declared ExecutionClusterID overrides RemediationRequest.Spec.ClusterID", func() {
		rr := buildRR("signal-origin-cluster")
		ai := buildAI("job", "gitops-hub-cluster")

		created := createAndGet(rr, ai)

		Expect(created.Spec.ClusterID).To(Equal("gitops-hub-cluster"),
			"BR-FLEET-004: the workflow's declared execution cluster must take precedence over the signal's origin cluster")
	})

	It("UT-RO-2326-002: falls back to RemediationRequest.Spec.ClusterID when the workflow declares no execution cluster (regression guard, unchanged default)", func() {
		rr := buildRR("signal-origin-cluster")
		ai := buildAI("job", "")

		created := createAndGet(rr, ai)

		Expect(created.Spec.ClusterID).To(Equal("signal-origin-cluster"),
			"unchanged default: execution runs on the signal's cluster when the workflow declares none")
	})

	It("UT-RO-2326-003: leaves ClusterID empty when neither the RR nor the workflow declare one (hub-local default, unchanged)", func() {
		rr := buildRR("")
		ai := buildAI("job", "")

		created := createAndGet(rr, ai)

		Expect(created.Spec.ClusterID).To(BeEmpty())
	})

	It("UT-RO-2326-004: propagates the workflow-declared cluster unchanged for the ansible engine (DD-FLEET-007 fail-closed regression pin)", func() {
		// RO creator applies no engine-specific special-casing to cluster
		// resolution -- AnsibleExecutor's own DD-FLEET-007 guard (#1761:
		// "ansible engine does not support remote execution") is what fails
		// this closed at dispatch time, purely by reading this same
		// WFE.Spec.ClusterID field. This test pins that RO creator keeps
		// producing that non-empty value for ansible exactly like it already
		// does for job/tekton, so that guard continues to trigger correctly
		// once a workflow declares an execution cluster.
		rr := buildRR("signal-origin-cluster")
		ai := buildAI("ansible", "edge-aggregator-cluster")

		created := createAndGet(rr, ai)

		Expect(created.Spec.ClusterID).To(Equal("edge-aggregator-cluster"),
			"the ansible engine must still receive the workflow-declared cluster on the spec -- AnsibleExecutor.Execute is the fail-closed enforcement point (DD-FLEET-007), not RO creator")
	})
})
