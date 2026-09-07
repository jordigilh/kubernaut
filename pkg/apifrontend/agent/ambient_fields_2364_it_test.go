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

package agent

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	k8sfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// IT #2364: proves fleet-hinted tool calls (ambient cluster_id/rr_id/session_id
// propagated by the model from console fleet hints and cross-phase
// preservation) survive the REAL ADK functiontool schema validation -- the
// exact step that killed 15 turns live. Mirrors the 2092 IT's verbatim
// Flow.callTool sequence: BeforeToolCallbacks first, then tool.Run on the
// same map, neither mocked.
var _ = Describe("IT #2364 — fleet-hinted calls through real ADK schema validation", func() {
	newToolCtx := func() statefulToolContext {
		state := newMapState()
		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "alice", Groups: []string{"sre"},
		})
		return statefulToolContext{
			fakeToolContext: fakeToolContext{Context: ctx},
			state:           state,
		}
	}

	It("IT-2364-001 (SI-10, AU-3): fleet-hinted present_decision survives real validation and presents", func() {
		presentTool, err := tools.NewPresentDecisionTool()
		Expect(err).NotTo(HaveOccurred())
		runnable, ok := presentTool.(runnableTool)
		Expect(ok).To(BeTrue())

		toolCtx := newToolCtx()
		before, after := NewPhaseGuardForTest()

		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-2364-it", "status": "completed",
			"summary": "Memory pressure on spoke worker nodes.",
		}, nil)

		args := map[string]any{
			"session_id": "sess-2364-it",
			"summary":    "Memory leak on spoke cluster worker.",
			"rca": map[string]any{
				"severity": "critical", "confidence": 0.9,
				"target": "Deployment/worker",
			},
			"options": []any{
				map[string]any{
					"workflow_id": "wf-1", "name": "spoke-memory-fix",
					"description": "Raise memory limits on the spoke",
				},
			},
			// Ambient fields the live model propagated; pre-fix ADK rejected
			// the call with: validating root: unexpected additional
			// properties ["cluster_id"] (14x) / ["rr_id" "cluster_id"] (1x).
			"cluster_id": "spoke",
			"rr_id":      "payments/rr-2364",
		}

		cbResult, cbErr := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(cbErr).NotTo(HaveOccurred())
		Expect(cbResult).To(BeNil(), "present_decision must still execute (AU-3 artifact mandate)")

		result, runErr := runnable.Run(toolCtx, args)
		Expect(runErr).NotTo(HaveOccurred(),
			"#2364: real ADK schema validation must accept fleet-hinted present_decision args")
		Expect(result["presented"]).To(BeTrue())
		message, _ := result["message"].(string)
		Expect(message).To(ContainSubstring("spoke-memory-fix"),
			"the decision must actually reach HandlePresentDecision, not just pass validation")
	})

	It("IT-2364-002 (SI-10, BR-FLEET-003): fleet-hinted discover/select survive real validation", func() {
		mockMCP := &ka.MockMCPClient{
			DiscoverWorkflowsFn: func(_ context.Context, _ ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
				return &ka.DiscoverWorkflowsResult{Workflows: []ka.DiscoveredWorkflow{
					{WorkflowID: "wf-1", Name: "spoke-memory-fix"},
				}}, nil
			},
			SelectWorkflowFn: func(_ context.Context, _ ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
				return &ka.SelectWorkflowResult{Status: "selected"}, nil
			},
		}

		toolCtx := newToolCtx()
		before, after := NewPhaseGuardForTest()

		// Successful investigate activates the driver session discover/select require.
		_, invErr := after(toolCtx, fakeTool{name: "kubernaut_investigate"},
			map[string]any{
				"rr_id":            "payments/rr-2364",
				"interaction_mode": session.InteractionModeFullRemediationAutonomous,
			},
			map[string]any{
				"session_id": "sess-2364-it", "rr_id": "payments/rr-2364",
				"status": "completed", "summary": "Memory pressure on spoke workers.",
			}, nil)
		Expect(invErr).NotTo(HaveOccurred())

		discoverTool, err := tools.NewDiscoverWorkflowsTool(mockMCP)
		Expect(err).NotTo(HaveOccurred())
		discoverRunnable, ok := discoverTool.(runnableTool)
		Expect(ok).To(BeTrue())

		discArgs := map[string]any{
			"rr_id": "payments/rr-2364",
			// Ambient fleet + preservation fields; pre-fix rejected.
			"cluster_id": "spoke",
			"session_id": "sess-2364-it",
		}
		cbResult, cbErr := before(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, discArgs)
		Expect(cbErr).NotTo(HaveOccurred())
		Expect(cbResult).To(BeNil())
		discResult, runErr := discoverRunnable.Run(toolCtx, discArgs)
		Expect(runErr).NotTo(HaveOccurred(),
			"#2364: real ADK schema validation must accept fleet-hinted discover_workflows args")
		Expect(discResult["count"]).To(BeEquivalentTo(1))

		selectTool, err := tools.NewSelectWorkflowTool(mockMCP, nil)
		Expect(err).NotTo(HaveOccurred())
		selectRunnable, ok := selectTool.(runnableTool)
		Expect(ok).To(BeTrue())

		selArgs := map[string]any{
			"rr_id": "payments/rr-2364", "workflow_id": "wf-1",
			"cluster_id": "spoke",
			"session_id": "sess-2364-it",
		}
		cbResult, cbErr = before(toolCtx, fakeTool{name: "kubernaut_select_workflow"}, selArgs)
		Expect(cbErr).NotTo(HaveOccurred())
		Expect(cbResult).To(BeNil())
		selResult, runErr := selectRunnable.Run(toolCtx, selArgs)
		Expect(runErr).NotTo(HaveOccurred(),
			"#2364: real ADK schema validation must accept fleet-hinted select_workflow args")
		Expect(selResult["status"]).To(Equal("selected"))
	})

	It("IT-2364-003 (SI-10): fleet-hinted watch survives real validation and reports terminal state", func() {
		scheme := k8sruntime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = eav1alpha1.AddToScheme(scheme)
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "rr-2364"},
			Spec: remediationv1.RemediationRequestSpec{
				TargetResource: remediationv1.ResourceIdentifier{Kind: "Deployment", Name: "worker"},
			},
		}
		rr.Status.OverallPhase = remediationv1.PhasePending
		wc := k8sfake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(rr).
			Build()

		watchTool, err := tools.NewWatchTool(wc, "payments")
		Expect(err).NotTo(HaveOccurred())
		runnable, ok := watchTool.(runnableTool)
		Expect(ok).To(BeTrue())

		toolCtx := newToolCtx()
		before, _ := NewPhaseGuardForTest()
		go func() {
			time.Sleep(50 * time.Millisecond)
			var current remediationv1.RemediationRequest
			Expect(wc.Get(context.Background(), crclient.ObjectKey{Namespace: "payments", Name: "rr-2364"}, &current)).To(Succeed())
			current.Status.OverallPhase = remediationv1.PhaseCompleted
			current.Status.EnsureCompletionStatus().Outcome = remediationv1.OutcomeRemediated
			current.Status.Message = "done"
			Expect(wc.Status().Update(context.Background(), &current)).To(Succeed())
		}()
		args := map[string]any{
			"name": "rr-2364",
			// Ambient fleet + preservation fields; pre-fix rejected.
			"cluster_id": "spoke",
			"session_id": "sess-2364-it",
		}
		cbResult, cbErr := before(toolCtx, fakeTool{name: "kubernaut_watch"}, args)
		Expect(cbErr).NotTo(HaveOccurred())
		Expect(cbResult).To(BeNil())

		result, runErr := runnable.Run(toolCtx, args)
		Expect(runErr).NotTo(HaveOccurred(),
			"#2364: real ADK schema validation must accept fleet-hinted watch args")
		Expect(result["status"]).To(Equal("completed"))
		Expect(result["outcome"]).To(Equal(remediationv1.OutcomeRemediated))
	})
})
