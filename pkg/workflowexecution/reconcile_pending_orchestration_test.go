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

package workflowexecution_test

// Characterization tests for reconcilePending's internal orchestration
// (dispatch/sequencing) logic, per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2
// (issue #1520). reconcilePending's individual sub-behaviors (ValidateSpec,
// BuildPipelineRun, MarkFailed, HandleAlreadyExists, updateStatus) already
// have extensive unit coverage (controller_test.go) and its terminal-failure
// dispatch paths are covered by controller_events_test.go (UT-WE-659-*).
// This file closes the two remaining orchestration branches that only had
// Integration-tier (envtest) coverage before decomposition, per AGENTS.md's
// mandate that unit tests verify business-level behavior tied to FedRAMP/SOC2
// control objectives (100% of business logic, structural line coverage):
//
//   - BR-WE-009 cooldown gating (FedRAMP SC-5: DoS Protection — prevents
//     redundant/concurrent executions from exhausting the execution namespace)
//   - BR-AUDIT-005 Gap #5 audit-emission idempotency (FedRAMP AU-2/AU-3,
//     SOC2 CC8.1 — the audit trail must be reconstructable without duplicate
//     workflow.selection.completed events when a reconcile re-enters Pending)
//
// These must stay green (behavior-preserving) across the Wave 2 decomposition
// of reconcilePending.

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/audit"
	weexecutor "github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/metrics"
	wephase "github.com/jordigilh/kubernaut/pkg/workflowexecution/phase"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/status"
)

var _ = Describe("reconcilePending orchestration [GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2]", func() {
	var (
		rpoScheme   *runtime.Scheme
		rpoRecorder *record.FakeRecorder
		rpoCtx      context.Context
	)

	BeforeEach(func() {
		rpoCtx = context.Background()
		rpoScheme = runtime.NewScheme()
		Expect(workflowexecutionv1alpha1.AddToScheme(rpoScheme)).To(Succeed())
		Expect(tektonv1.AddToScheme(rpoScheme)).To(Succeed())
		Expect(corev1.AddToScheme(rpoScheme)).To(Succeed())
		rpoRecorder = record.NewFakeRecorder(20)
	})

	targetResourceIndexer := func(obj client.Object) []string {
		return []string{obj.(*workflowexecutionv1alpha1.WorkflowExecution).Spec.TargetResource}
	}

	buildPendingWFE := func(name, targetResource string) *workflowexecutionv1alpha1.WorkflowExecution {
		return &workflowexecutionv1alpha1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "default",
				UID:        types.UID("uid-" + name),
				Finalizers: []string{workflowexecution.FinalizerName},
			},
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				TargetResource: targetResource,
				WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
					WorkflowID:      "wf-" + name,
					Version:         "v1",
					ExecutionBundle: "ghcr.io/kubernaut/workflows/restart:v1.0.0",
					// Issue #1661 Change 11e: engine is read directly from
					// WorkflowRef, not resolved via a DataStorage round-trip
					// at runtime.
					ExecutionEngine: "tekton",
				},
			},
		}
	}

	buildReconciler := func(fakeClient client.Client, registry *weexecutor.Registry, mockStore *mockAuditStore) *workflowexecution.WorkflowExecutionReconciler {
		am := audit.NewManager(mockStore, logr.Discard())
		sm := status.NewManager(fakeClient)
		tm := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		pm := wephase.NewManager()

		return &workflowexecution.WorkflowExecutionReconciler{
			Client:             fakeClient,
			APIReader:          fakeClient,
			Scheme:             rpoScheme,
			Recorder:           rpoRecorder,
			ExecutionNamespace: "kubernaut-workflows",
			CooldownPeriod:     5 * time.Minute,
			AuditStore:         mockStore,
			Metrics:            tm,
			StatusManager:      sm,
			AuditManager:       am,
			PhaseManager:       pm,
			ExecutorRegistry:   registry,
		}
	}

	// BR-WE-009: Cooldown Gating
	// FedRAMP SC-5 (DoS Protection): blocking re-execution against a target
	// resource still inside its cooldown window prevents unbounded concurrent
	// executions from exhausting the execution namespace / target resource.
	Context("UT-WE-1520-001 (P0): Cooldown gating blocks execution and requeues (BR-WE-009, FedRAMP SC-5)", func() {
		It("should stay Pending, requeue after remaining cooldown, and emit CooldownActive instead of creating an execution resource", func() {
			targetResource := "default/deployment/cooldown-target"

			// A prior WFE against the same target completed 1 minute ago — well
			// inside the 5-minute default cooldown window.
			priorWFE := buildPendingWFE("wfe-cooldown-prior", targetResource)
			priorWFE.Status.Phase = workflowexecutionv1alpha1.PhaseCompleted
			priorWFE.Status.CompletionTime = &metav1.Time{Time: time.Now().Add(-1 * time.Minute)}

			wfe := buildPendingWFE("wfe-cooldown-current", targetResource)

			fakeClient := fake.NewClientBuilder().
				WithScheme(rpoScheme).
				WithObjects(priorWFE, wfe).
				WithStatusSubresource(wfe, priorWFE).
				WithIndex(&workflowexecutionv1alpha1.WorkflowExecution{}, "spec.targetResource", targetResourceIndexer).
				Build()

			tektonExec := weexecutor.NewTektonExecutor(fakeClient)
			registry := weexecutor.NewRegistry()
			registry.Register(tektonExec.Engine(), tektonExec)
			mockStore := &mockAuditStore{events: make([]*ogenclient.AuditEventRequest, 0)}

			reconciler := buildReconciler(fakeClient, registry, mockStore)

			for len(rpoRecorder.Events) > 0 {
				<-rpoRecorder.Events
			}

			result, err := reconciler.Reconcile(rpoCtx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: wfe.Name, Namespace: wfe.Namespace},
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"cooldown-blocked reconcile must requeue after the remaining cooldown, not error or drop the WFE")
			Expect(result.RequeueAfter).To(BeNumerically("<=", 5*time.Minute),
				"remaining cooldown must not exceed the configured cooldown period")

			var evts []string
			for len(rpoRecorder.Events) > 0 {
				evts = append(evts, <-rpoRecorder.Events)
			}
			Expect(hasEventMatch(evts, "Normal", events.EventReasonCooldownActive)).
				To(BeTrue(), "cooldown gating must emit CooldownActive (BR-WE-009), got: %v", evts)

			Expect(mockStore.events).To(BeEmpty(),
				"no workflow.selection.completed audit event must be emitted while blocked by cooldown (BR-AUDIT-005 idempotency)")

			var fetched workflowexecutionv1alpha1.WorkflowExecution
			Expect(fakeClient.Get(rpoCtx, types.NamespacedName{Name: wfe.Name, Namespace: wfe.Namespace}, &fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhasePending),
				"WFE must remain in Pending phase while cooldown-gated, not silently transition")
		})
	})

	// BR-AUDIT-005 Gap #5: Audit-Emission Idempotency
	// FedRAMP AU-2/AU-3 (Audit Events / Content), SOC2 CC8.1 (audit
	// completeness): a duplicate workflow.selection.completed event on
	// reconcile re-entry would corrupt the SOC2 CC8.1 remediation-request
	// reconstruction (multiple "selection completed" events for one WFE).
	Context("UT-WE-1520-002 (P0): Audit idempotency skips duplicate selection-completed event when execution resource already exists (BR-AUDIT-005 Gap #5, FedRAMP AU-2/AU-3, SOC2 CC8.1)", func() {
		It("should not re-emit workflow.selection.completed when the execution PipelineRun already exists", func() {
			targetResource := "default/deployment/idempotent-target"
			wfe := buildPendingWFE("wfe-audit-idempotent", targetResource)

			// Pre-create the execution resource this WFE would create, at the
			// exact deterministic name reconcilePending computes, simulating a
			// reconcile that re-enters Pending after the resource was already
			// created by a prior pass (informer cache lag / requeue).
			resourceName := weexecutor.ExecutionResourceName(targetResource)
			existingPR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "kubernaut-workflows",
					// Labels matching this WFE's ownership markers (per
					// HandleAlreadyExists) simulate a prior successful create by
					// THIS WFE, not a collision with a different one.
					Labels: map[string]string{
						"kubernaut.ai/workflow-execution": wfe.Name,
						"kubernaut.ai/source-namespace":   wfe.Namespace,
					},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(rpoScheme).
				WithObjects(wfe, existingPR).
				WithStatusSubresource(wfe).
				WithIndex(&workflowexecutionv1alpha1.WorkflowExecution{}, "spec.targetResource", targetResourceIndexer).
				Build()

			tektonExec := weexecutor.NewTektonExecutor(fakeClient)
			registry := weexecutor.NewRegistry()
			registry.Register(tektonExec.Engine(), tektonExec)
			mockStore := &mockAuditStore{events: make([]*ogenclient.AuditEventRequest, 0)}

			reconciler := buildReconciler(fakeClient, registry, mockStore)

			var sanityPR tektonv1.PipelineRun
			Expect(fakeClient.Get(rpoCtx, types.NamespacedName{Name: resourceName, Namespace: "kubernaut-workflows"}, &sanityPR)).To(Succeed(),
				"sanity check: pre-seeded PipelineRun %s must be gettable before Reconcile", resourceName)

			_, err := reconciler.Reconcile(rpoCtx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: wfe.Name, Namespace: wfe.Namespace},
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(mockStore.events).To(BeEmpty(),
				"workflow.selection.completed must be skipped once the execution resource already exists (BR-AUDIT-005 idempotency, prevents SOC2 CC8.1 duplicate-event corruption), got: %d event(s)", len(mockStore.events))
		})

		It("should emit workflow.selection.completed exactly once when the execution resource does not yet exist", func() {
			targetResource := "default/deployment/first-creation-target"
			wfe := buildPendingWFE("wfe-audit-first", targetResource)

			fakeClient := fake.NewClientBuilder().
				WithScheme(rpoScheme).
				WithObjects(wfe).
				WithStatusSubresource(wfe).
				WithIndex(&workflowexecutionv1alpha1.WorkflowExecution{}, "spec.targetResource", targetResourceIndexer).
				Build()

			tektonExec := weexecutor.NewTektonExecutor(fakeClient)
			registry := weexecutor.NewRegistry()
			registry.Register(tektonExec.Engine(), tektonExec)
			mockStore := &mockAuditStore{events: make([]*ogenclient.AuditEventRequest, 0)}

			reconciler := buildReconciler(fakeClient, registry, mockStore)

			_, err := reconciler.Reconcile(rpoCtx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: wfe.Name, Namespace: wfe.Namespace},
			})
			Expect(err).ToNot(HaveOccurred())

			selectionCompletedCount := 0
			for _, e := range mockStore.events {
				if e.EventType == "workflowexecution.selection.completed" {
					selectionCompletedCount++
				}
			}
			Expect(selectionCompletedCount).To(Equal(1),
				"workflow.selection.completed must be emitted exactly once on first execution-resource creation (BR-AUDIT-005), got: %d occurrence(s) among %d total audit event(s)", selectionCompletedCount, len(mockStore.events))
		})
	})
})
