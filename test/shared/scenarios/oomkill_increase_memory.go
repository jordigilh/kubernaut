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

package scenarios

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Gomega DSL convention

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// OOMKillIncreaseMemoryScenarioConfig holds the live clients/config needed to
// drive and assert the oomkill-increase-memory-v1 workflow end-to-end.
//
// Issue #1542 follow-up: shared between the FullPipeline (single-cluster,
// real OOMKill trigger) and Fleet (cross-cluster, synthetic cluster-tagged
// alert trigger) E2E lanes so both prove the SAME real, verifiable fix
// instead of only FP asserting the outcome. Mirrors
// CrashLoopConfigFixScenarioConfig's shape (test/shared/scenarios/
// crashloop_config_fix.go).
type OOMKillIncreaseMemoryScenarioConfig struct {
	// Ctx is the base context for CRD polling (always the hub cluster).
	Ctx context.Context

	// CRDClient reads RemediationRequest/SignalProcessing/AIAnalysis/
	// WorkflowExecution/EffectivenessAssessment CRDs. Always the hub cluster.
	CRDClient client.Reader

	// TargetClient reads/verifies the memory-eater Deployment/Pods. For FP
	// this is the same (single) cluster as CRDClient; for Fleet this is the
	// remote cluster's client (DD-TEST-013).
	TargetClient client.Client

	// JobClient reads the K8s Job the WE Job executor creates. For FP this
	// is the hub cluster (same as CRDClient); for Fleet, WE's
	// mcpClientFactory routes Job creation to the REMOTE cluster whenever
	// RemediationRequest.ClusterID is set (pkg/workflowexecution/executor/
	// client_factory.go), so this must be the remote cluster's client
	// (typically the same value as TargetClient).
	JobClient client.Client

	// CRDNamespace is the namespace RR/SP/AA/WFE/EA CRDs live in
	// (kubernaut-system, per ADR-057).
	CRDNamespace string

	// TargetNamespace is the namespace hosting the memory-eater fixture.
	TargetNamespace string

	// TargetDeploymentName is the name of the memory-eater Deployment
	// (DeployMemoryEaterNamed lets callers avoid colliding with any
	// shared/suite-level "memory-eater" fixture).
	TargetDeploymentName string

	// JobNamespace is where the WE Job executor creates the K8s Job
	// (kubernaut-workflows), on whichever cluster JobClient targets.
	JobNamespace string

	// RemediationRequestName identifies the RR created by the caller's
	// trigger step (real OOMKill for FP, synthetic alert POST for Fleet).
	RemediationRequestName string

	// ExpectClusterID, when non-empty, asserts RR.Spec.ClusterID equals this
	// value (fleet lane cross-cluster proof).
	ExpectClusterID string

	// ExpectedWorkflowID, when non-empty, asserts AA selected exactly this
	// workflow (the real DataStorage-assigned UUID for
	// oomkill-increase-memory-v1, resolved by the caller from the
	// suite-level workflowUUIDs map).
	ExpectedWorkflowID string

	// ExpectedMemoryLimit is the new memory limit the real fix should patch
	// the target Deployment to (e.g. "512Mi", matching mock-llm's
	// oomkilledConfig() MEMORY_LIMIT_NEW parameter).
	ExpectedMemoryLimit string

	Timeout  time.Duration
	Interval time.Duration
}

// RunOOMKillIncreaseMemoryScenario drives and asserts the shared outcome for
// Issue #1542 follow-up: RR -> SP -> AA (selects oomkill-increase-memory-v1)
// -> WE (job engine) -> K8s Job -> real memory-limit patch + Deployment
// recovery -> EA health score > 0 (hard assertion, not simulated).
//
// The caller is responsible for triggering the signal (real OOMKill for FP,
// synthetic cluster-tagged alert for Fleet) and resolving
// RemediationRequestName before calling this function.
func RunOOMKillIncreaseMemoryScenario(cfg OOMKillIncreaseMemoryScenarioConfig) {
	ctx := cfg.Ctx
	GinkgoHelper()

	By("Verifying RemediationRequest target resource and cluster identity")
	rr := &remediationv1.RemediationRequest{}
	Expect(cfg.CRDClient.Get(ctx, client.ObjectKey{
		Name: cfg.RemediationRequestName, Namespace: cfg.CRDNamespace,
	}, rr)).To(Succeed(), "RemediationRequest must exist")
	if cfg.ExpectClusterID != "" {
		Expect(rr.Spec.ClusterID).To(Equal(cfg.ExpectClusterID),
			"RR must carry the fleet remote cluster identity (BR-FLEET-054)")
	}

	By("Waiting for SignalProcessing to complete")
	Eventually(func() string {
		spList := &signalprocessingv1.SignalProcessingList{}
		if err := cfg.CRDClient.List(ctx, spList, client.InNamespace(cfg.CRDNamespace)); err != nil {
			return ""
		}
		for i := range spList.Items {
			sp := &spList.Items[i]
			if sp.Spec.RemediationRequestRef.Name == cfg.RemediationRequestName {
				return string(sp.Status.Phase)
			}
		}
		return ""
	}, cfg.Timeout, cfg.Interval).Should(Equal("Completed"),
		"SignalProcessing should reach Completed phase")

	By("Waiting for AIAnalysis to select oomkill-increase-memory-v1 (job engine)")
	var aaName string
	Eventually(func() string {
		aaList := &aianalysisv1.AIAnalysisList{}
		if err := cfg.CRDClient.List(ctx, aaList, client.InNamespace(cfg.CRDNamespace)); err != nil {
			return ""
		}
		for i := range aaList.Items {
			aa := &aaList.Items[i]
			if aa.Spec.RemediationRequestRef.Name == cfg.RemediationRequestName {
				aaName = aa.Name
				return aa.Status.Phase
			}
		}
		return ""
	}, cfg.Timeout, cfg.Interval).Should(Equal("Completed"),
		"AIAnalysis should reach Completed phase")

	aa := &aianalysisv1.AIAnalysis{}
	Expect(cfg.CRDClient.Get(ctx, client.ObjectKey{Name: aaName, Namespace: cfg.CRDNamespace}, aa)).To(Succeed())
	Expect(aa.Status.SelectedWorkflow).ToNot(BeNil(), "AIAnalysis should have selected a workflow")
	Expect(aa.Status.SelectedWorkflow.ExecutionEngine).To(Equal("job"),
		"oomkill-increase-memory-v1 uses the job execution engine")
	if cfg.ExpectedWorkflowID != "" {
		Expect(aa.Status.SelectedWorkflow.WorkflowID).To(Equal(cfg.ExpectedWorkflowID),
			"AA must select the real oomkill-increase-memory-v1 workflow (not a generic fallback)")
	}
	Expect(aa.Status.SelectedWorkflow.Parameters).To(HaveKey("MEMORY_LIMIT_NEW"),
		"selected workflow must carry the memory-limit fix parameter (proves oomkill-increase-memory-v1, not a different workflow)")

	By("Waiting for WorkflowExecution to be created with job execution engine")
	var weName string
	Eventually(func() string {
		weList := &workflowexecutionv1.WorkflowExecutionList{}
		if err := cfg.CRDClient.List(ctx, weList, client.InNamespace(cfg.CRDNamespace)); err != nil {
			return ""
		}
		for i := range weList.Items {
			we := &weList.Items[i]
			if we.Spec.RemediationRequestRef.Name == cfg.RemediationRequestName {
				weName = we.Name
				return we.Status.ExecutionEngine
			}
		}
		return ""
	}, cfg.Timeout, cfg.Interval).Should(Equal("job"),
		"WorkflowExecution should use job execution engine")

	By("Waiting for the K8s Job to complete successfully (real remediate.sh execution)")
	Eventually(func(g Gomega) {
		we := &workflowexecutionv1.WorkflowExecution{}
		if getErr := cfg.CRDClient.Get(ctx, client.ObjectKey{Name: weName, Namespace: cfg.CRDNamespace}, we); getErr == nil {
			g.Expect(we.Status.Phase).NotTo(Equal("Failed"),
				fmt.Sprintf("WorkflowExecution %s reached Failed phase (reason: %s)", weName, we.Status.FailureReason))
		}

		jobList := &batchv1.JobList{}
		g.Expect(cfg.JobClient.List(ctx, jobList,
			client.InNamespace(cfg.JobNamespace),
			client.MatchingLabels{"kubernaut.ai/workflow-execution": weName})).To(Succeed())
		g.Expect(jobList.Items).NotTo(BeEmpty(), "No Jobs found for WorkflowExecution %s", weName)

		job := jobList.Items[0]
		g.Expect(job.Status.Failed).To(BeZero(),
			fmt.Sprintf("Job %s has %d failed pod(s) — real remediate.sh execution failed", job.Name, job.Status.Failed))
		g.Expect(job.Status.Succeeded).To(BeNumerically(">", 0),
			fmt.Sprintf("Job %s has not succeeded yet (active=%d)", job.Name, job.Status.Active))
	}, cfg.Timeout, cfg.Interval).Should(Succeed(), "K8s Job should complete successfully")

	By("Verifying the Deployment's memory limit was actually patched (real fix, not a simulation)")
	wantQty := resource.MustParse(cfg.ExpectedMemoryLimit)
	Eventually(func(g Gomega) {
		dep := &appsv1.Deployment{}
		g.Expect(cfg.TargetClient.Get(ctx, client.ObjectKey{
			Name: cfg.TargetDeploymentName, Namespace: cfg.TargetNamespace,
		}, dep)).To(Succeed())
		g.Expect(dep.Spec.Template.Spec.Containers).NotTo(BeEmpty())
		gotQty := dep.Spec.Template.Spec.Containers[0].Resources.Limits[corev1.ResourceMemory]
		g.Expect(gotQty.Cmp(wantQty)).To(Equal(0),
			"remediate.sh must patch the memory limit to %s (got %s)", wantQty.String(), gotQty.String())
	}, cfg.Timeout, cfg.Interval).Should(Succeed())

	By("Waiting for WorkflowExecution to reach Completed phase")
	Eventually(func() string {
		we := &workflowexecutionv1.WorkflowExecution{}
		if err := cfg.CRDClient.Get(ctx, client.ObjectKey{Name: weName, Namespace: cfg.CRDNamespace}, we); err != nil {
			return ""
		}
		return we.Status.Phase
	}, cfg.Timeout, cfg.Interval).Should(Equal("Completed"),
		"WorkflowExecution should reach Completed phase")

	By("Verifying the memory-eater Deployment recovered (no pod remains in CrashLoopBackOff, container Ready)")
	Eventually(func(g Gomega) {
		pods := &corev1.PodList{}
		g.Expect(cfg.TargetClient.List(ctx, pods, client.InNamespace(cfg.TargetNamespace),
			client.MatchingLabels{"app": cfg.TargetDeploymentName})).To(Succeed())
		g.Expect(pods.Items).NotTo(BeEmpty(), "memory-eater pods should exist")
		for _, pod := range pods.Items {
			g.Expect(pod.Status.Phase).To(Equal(corev1.PodRunning),
				"pod %s must be Running after remediation", pod.Name)
			ready := false
			for _, cs := range pod.Status.ContainerStatuses {
				g.Expect(cs.State.Waiting).To(Or(BeNil(), Not(HaveField("Reason", Equal("CrashLoopBackOff")))),
					"pod %s must not remain in CrashLoopBackOff after remediation", pod.Name)
				if cs.Ready {
					ready = true
				}
			}
			g.Expect(ready).To(BeTrue(), "pod %s container must be Ready after remediation", pod.Name)
		}
	}, cfg.Timeout, cfg.Interval).Should(Succeed(), "memory-eater deployment should recover to a stable Ready state")

	By("Waiting for RemediationRequest to reach Completed phase")
	Eventually(func() string {
		fetched := &remediationv1.RemediationRequest{}
		if err := cfg.CRDClient.Get(ctx, client.ObjectKey{
			Name: cfg.RemediationRequestName, Namespace: cfg.CRDNamespace,
		}, fetched); err != nil {
			return ""
		}
		return string(fetched.Status.OverallPhase)
	}, cfg.Timeout, cfg.Interval).Should(Equal("Completed"),
		"RemediationRequest should reach Completed phase")

	By("Verifying EffectivenessAssessment reports a positive health score (hard assertion — real recovery)")
	eaName := fmt.Sprintf("ea-%s", cfg.RemediationRequestName)
	eaKey := client.ObjectKey{Name: eaName, Namespace: cfg.CRDNamespace}
	ea := &eav1.EffectivenessAssessment{}
	Eventually(func() string {
		if err := cfg.CRDClient.Get(ctx, eaKey, ea); err != nil {
			return ""
		}
		return ea.Status.Phase
	}, cfg.Timeout, cfg.Interval).Should(BeElementOf(eav1.PhaseCompleted, eav1.PhaseFailed),
		"EA should reach terminal phase")

	Expect(cfg.CRDClient.Get(ctx, eaKey, ea)).To(Succeed())
	Expect(ea.Status.Components.HealthAssessed).To(BeTrue(), "EA health component should be assessed")
	Expect(ea.Status.Components.HealthScore).ToNot(BeNil(), "EA HealthScore should be populated")
	Expect(*ea.Status.Components.HealthScore).To(BeNumerically(">", 0),
		"Issue #1542 follow-up: HealthScore must be > 0 — the memory-eater deployment genuinely recovered "+
			"after the real memory-limit fix, not a simulated no-op")
}
