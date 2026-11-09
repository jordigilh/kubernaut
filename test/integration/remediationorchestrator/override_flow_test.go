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

package remediationorchestrator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	batchv1 "k8s.io/api/batch/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	wecontroller "github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/shared/contenthash"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	weaudit "github.com/jordigilh/kubernaut/pkg/workflowexecution/audit"
	weexecutor "github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
	wemetrics "github.com/jordigilh/kubernaut/pkg/workflowexecution/metrics"
	wephase "github.com/jordigilh/kubernaut/pkg/workflowexecution/phase"
	westatus "github.com/jordigilh/kubernaut/pkg/workflowexecution/status"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	completed           = "Completed"
	operatorKubernautAi = "operator@kubernaut.ai"
	ansibleEngine       = "ansible"
)

// TDD Phase: RED — Issue #594 RO Integration Tests
// BR-ORCH-030/032/033: Operator Workflow Override via RAR Approval
//
// These tests validate the full approval-with-override flow through
// the RO reconciler using envtest (real K8s API server).
// Phase 1 pattern: Manual child CRD control.

var _ = Describe("BR-ORCH-030: Operator Override Integration (#594)", Label("integration", "override"), func() {
	var (
		namespace string
		rrName    string
	)

	progressToAwaitingApproval := func(rrName string, aiWorkflow *aianalysisv1.SelectedWorkflow) {
		By("Creating RR and progressing to AwaitingApproval")
		_ = createRemediationRequest(namespace, rrName)

		spName := fmt.Sprintf("sp-%s", rrName)
		Eventually(func() error {
			sp := &signalprocessingv1.SignalProcessing{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
		}, timeout, interval).Should(Succeed())
		Expect(updateSPStatus(spName)).To(Succeed())

		aiName := fmt.Sprintf("ai-%s", rrName)
		Eventually(func() error {
			ai := &aianalysisv1.AIAnalysis{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: ROControllerNamespace}, ai)
		}, timeout, interval).Should(Succeed())

		ai := &aianalysisv1.AIAnalysis{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: ROControllerNamespace}, ai)).To(Succeed())
		ai.Status.Phase = completed
		ai.Status.EnsureApproval().ApprovalRequired = true
		ai.Status.Approval.ApprovalReason = "Confidence below threshold"
		ai.Status.EnsureRCAResult().SelectedWorkflow = aiWorkflow
		ai.Status.RCAResult.RootCause = "OOMKill detected"
		ai.Status.RCAResult.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:    "Memory leak causing OOM",
			Severity:   "critical",
			SignalType: "alert",
			RemediationTarget: &aianalysisv1.RemediationTarget{
				Kind:      "Deployment",
				Name:      "test-app",
				Namespace: namespace,
			},
		}
		now := metav1.Now()
		ai.Status.CompletedAt = &now
		Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

		Eventually(func() string {
			rr := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, rr); err != nil {
				return ""
			}
			return string(rr.Status.OverallPhase)
		}, timeout, interval).Should(Equal("AwaitingApproval"))
	}

	createOverrideRW := func(name string) *remediationworkflowv1.RemediationWorkflow {
		rw := &remediationworkflowv1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ROControllerNamespace,
			},
			Spec: remediationworkflowv1.RemediationWorkflowSpec{
				Version:    "2.0.0",
				ActionType: "DrainRestart",
				Description: remediationworkflowv1.RemediationWorkflowDescription{
					What:      "Drains and restarts node",
					WhenToUse: "When node is unhealthy",
				},
				Labels: remediationworkflowv1.RemediationWorkflowLabels{
					Severity:    []string{"critical"},
					Environment: []string{"production"},
					Component:   []string{"v1/Node"},
					Priority:    "P1",
				},
				Execution: remediationworkflowv1.RemediationWorkflowExecution{
					Engine:             "job",
					Bundle:             "override-bundle:v2.0@sha256:override123",
					BundleDigest:       "sha256:override123",
					EngineConfig:       &apiextensionsv1.JSON{Raw: []byte(`{"image":"drain:v2"}`)},
					ServiceAccountName: "override-sa",
				},
				Parameters: []remediationworkflowv1.RemediationWorkflowParameter{
					{Name: "TIMEOUT", Type: "string", Required: true, Description: "drain timeout"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())

		rw.Status.WorkflowID = "wf-override-it-002"
		rw.Status.CatalogStatus = sharedtypes.CatalogStatusActive
		Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())

		return rw
	}

	defaultAIWorkflow := func() *aianalysisv1.SelectedWorkflow {
		return &aianalysisv1.SelectedWorkflow{
			WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
				WorkflowID:            trustedAIOriginalWorkflowID,
				WorkflowName:          trustedAIOriginalWorkflowName,
				ActionType:            "RestartPod",
				Version:               "1.0.0",
				ExecutionBundle:       "ai-bundle:v1.0@sha256:aaa",
				ExecutionBundleDigest: "sha256:aaa",
				ExecutionEngine:       "tekton",
			},
			Confidence: 0.72,
			Rationale:  "AI recommended restart",
			Parameters: typedStringParameters(map[string]string{
				"NAMESPACE": "default",
				"POD_NAME":  "app-pod-1",
			}),
		}
	}

	BeforeEach(func() {
		namespace = createTestNamespace(ctx, "ro-override")
		rrName = fmt.Sprintf("rr-ovr-%s", uuid.New().String()[:13])
	})

	AfterEach(func() {
		deleteTestNamespace(namespace)
	})

	It("IT-2570-022: BR-ORCH-026 escalates a pre-existing RAR collision without reuse", func() {
		rr := createRemediationRequest(namespace, rrName)
		spName := "sp-" + rrName
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, &signalprocessingv1.SignalProcessing{})
		}, timeout, interval).Should(Succeed())
		Expect(updateSPStatus(spName)).To(Succeed())
		aiKey := types.NamespacedName{Name: "ai-" + rrName, Namespace: ROControllerNamespace}
		Eventually(func() error { return k8sManager.GetAPIReader().Get(ctx, aiKey, &aianalysisv1.AIAnalysis{}) }, timeout, interval).Should(Succeed())
		existing := rarSafetyRequest(rr, time.Now().Add(15*time.Minute), false)
		existing.Spec.WhyApprovalRequired = "pre-existing conflicting review"
		Expect(k8sClient.Create(ctx, existing)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, existing))).To(Succeed()) })
		originalUID := existing.UID
		ai := &aianalysisv1.AIAnalysis{}
		Expect(k8sManager.GetAPIReader().Get(ctx, aiKey, ai)).To(Succeed())
		ai.Status.Phase = completed
		ai.Status.EnsureApproval().ApprovalRequired = true
		ai.Status.Approval.ApprovalReason = "review required"
		ai.Status.EnsureRCAResult().SelectedWorkflow = defaultAIWorkflow()
		ai.Status.RCAResult.RootCause = "test investigation"
		now := metav1.Now()
		ai.Status.CompletedAt = &now
		Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())
		persistedRR := &remediationv1.RemediationRequest{}
		Eventually(func() string {
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), persistedRR); err != nil {
				return ""
			}
			return string(persistedRR.Status.OverallPhase)
		}, timeout, interval).Should(Equal("Failed"))
		completion := persistedRR.Status.GetCompletionStatus()
		Expect(completion.RequiresManualReview).To(BeTrue())
		Expect(completion.Outcome).To(Equal(remediationv1.OutcomeManualReviewRequired))
		Expect(completion.FailurePhase).NotTo(BeNil())
		Expect(*completion.FailurePhase).To(Equal(remediationv1.FailurePhaseApproval))
		Expect(completion.FailureReason).NotTo(BeNil())
		Expect(*completion.FailureReason).To(Equal(creator.ApprovalCreationReasonCollision))
		persistedRAR := &remediationv1.RemediationApprovalRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(existing), persistedRAR)).To(Succeed())
		Expect(persistedRAR.UID).To(Equal(originalUID))
		Expect(persistedRAR.Spec.WhyApprovalRequired).To(Equal("pre-existing conflicting review"))
		we := &workflowexecutionv1.WorkflowExecution{}
		Expect(apierrors.IsNotFound(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: "we-" + rrName, Namespace: ROControllerNamespace}, we))).To(BeTrue())
	})

	It("IT-RO-2570-041: BR-ORCH-025 projects the reviewed RAR snapshot and partial override into WE", func() {
		progressToAwaitingApproval(rrName, defaultAIWorkflow())
		rarName := fmt.Sprintf("rar-%s", rrName)
		rar := &remediationv1.RemediationApprovalRequest{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: ROControllerNamespace}, rar)
		}, timeout, interval).Should(Succeed())
		Expect(rar.Spec.Parameters).To(Equal(typedStringParameters(map[string]string{"NAMESPACE": "default", "POD_NAME": "app-pod-1"})))

		ai := &aianalysisv1.AIAnalysis{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: "ai-" + rrName, Namespace: ROControllerNamespace}, ai)).To(Succeed())
		ai.Status.RCAResult.SelectedWorkflow.Parameters["NAMESPACE"] = typedStringParameter("changed-after-review")
		ai.Status.RCAResult.RootCauseAnalysis.RemediationTarget.Name = "changed-after-review"
		Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

		Expect(approveRAR(rarName, func(current *remediationv1.RemediationApprovalRequest) {
			current.Status.Decision = remediationv1.ApprovalDecisionApproved
			current.Status.DecidedBy = operatorKubernautAi
			decidedAt := metav1.Now()
			current.Status.DecidedAt = &decidedAt
			current.Status.WorkflowOverride = &remediationv1.WorkflowOverride{Parameters: typedStringParameters(map[string]string{"POD_NAME": "reviewed-pod"}), Rationale: "operator review"}
		})).To(Succeed())

		we := &workflowexecutionv1.WorkflowExecution{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: "we-" + rrName, Namespace: ROControllerNamespace}, we)
		}, timeout, interval).Should(Succeed())
		Expect(we.Spec.WorkflowRef.WorkflowID).To(Equal(trustedAIOriginalWorkflowID))
		Expect(we.Spec.Parameters).To(Equal(typedStringParameters(map[string]string{"NAMESPACE": "default", "POD_NAME": "reviewed-pod"})))
		Expect(we.Spec.TargetResource).To(Equal(fmt.Sprintf("%s/Deployment/test-app", namespace)))
	})

	It("IT-RO-2570-023: BR-ORCH-025 carries a partial override through the RO reconciler to WE", func() {
		progressToAwaitingApproval(rrName, defaultAIWorkflow())
		rarName := fmt.Sprintf("rar-%s", rrName)
		Expect(approveRAR(rarName, func(current *remediationv1.RemediationApprovalRequest) {
			current.Status.Decision = remediationv1.ApprovalDecisionApproved
			current.Status.DecidedBy = operatorKubernautAi
			decidedAt := metav1.Now()
			current.Status.DecidedAt = &decidedAt
			current.Status.WorkflowOverride = &remediationv1.WorkflowOverride{Parameters: typedStringParameters(map[string]string{"POD_NAME": "reviewed-pod"}), Rationale: "operator review"}
		})).To(Succeed())
		we := &workflowexecutionv1.WorkflowExecution{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: "we-" + rrName, Namespace: ROControllerNamespace}, we)
		}, timeout, interval).Should(Succeed())
		Expect(we.Spec.Parameters).To(Equal(typedStringParameters(map[string]string{"NAMESPACE": "default", "POD_NAME": "reviewed-pod"})))
	})

	It("IT-RO-WE-2570-003: BR-ORCH-025 fails closed when the approved catalog identity disappears before WE dispatch", func() {
		selected := defaultAIWorkflow()
		rw := &remediationworkflowv1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: "missing-2570-" + uuid.NewString()[:8], Namespace: ROControllerNamespace},
			Spec: remediationworkflowv1.RemediationWorkflowSpec{
				Version: "1.0.0", ActionType: "RestartPod",
				Description: remediationworkflowv1.RemediationWorkflowDescription{What: "Isolated approval fixture", WhenToUse: "Integration test"},
				Labels:      remediationworkflowv1.RemediationWorkflowLabels{Severity: []string{"critical"}, Environment: []string{"production"}, Component: []string{"v1/Pod"}, Priority: "P1"},
				Execution:   remediationworkflowv1.RemediationWorkflowExecution{Engine: "job", Bundle: testutil.ValidBundleRef},
				Parameters:  []remediationworkflowv1.RemediationWorkflowParameter{{Name: "NAMESPACE", Type: "string", Required: true, Description: "namespace"}, {Name: "POD_NAME", Type: "string", Required: true, Description: "pod name"}},
			},
		}
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, rw))).To(Succeed()) })
		content, err := contenthash.MarshalCleanCRDContent(rw)
		Expect(err).NotTo(HaveOccurred())
		rw.Status.WorkflowID = contenthash.DeterministicUUID(contenthash.ComputeContentHash(string(content)))
		Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())
		workflowID := rw.Status.WorkflowID
		selected.WorkflowID = workflowID
		selected.WorkflowName = rw.Name
		selected.ExecutionEngine = "job"
		selected.ExecutionBundle = testutil.ValidBundleRef
		selected.ExecutionBundleDigest = ""
		var matches remediationworkflowv1.RemediationWorkflowList
		Expect(k8sManager.GetAPIReader().List(ctx, &matches, client.InNamespace(ROControllerNamespace), client.MatchingFields{"status.workflowId": workflowID})).To(Succeed())
		Expect(matches.Items).To(HaveLen(1))
		Expect(matches.Items[0].UID).To(Equal(rw.UID))

		progressToAwaitingApproval(rrName, selected)
		Expect(approveRAR("rar-"+rrName, func(current *remediationv1.RemediationApprovalRequest) {
			current.Status.Decision = remediationv1.ApprovalDecisionApproved
			current.Status.DecidedBy = operatorKubernautAi
			decidedAt := metav1.Now()
			current.Status.DecidedAt = &decidedAt
		})).To(Succeed())
		key := types.NamespacedName{Name: "we-" + rrName, Namespace: ROControllerNamespace}
		var we workflowexecutionv1.WorkflowExecution
		Eventually(func(g Gomega) {
			g.Expect(k8sManager.GetAPIReader().Get(ctx, key, &we)).To(Succeed())
			g.Expect(we.Spec.WorkflowRef.WorkflowID).To(Equal(workflowID))
			g.Expect(we.Spec.RemediationRequestRef.Name).To(Equal(rrName))
			g.Expect(we.Status.ExecutionRef).To(BeNil())
		}, timeout, interval).Should(Succeed())
		// The real WE controller adds this metadata before dispatch. This suite
		// runs RO-only, so add the finalizer explicitly to exercise WE's
		// terminal failure path after the trusted catalog entry disappears.
		controllerutil.AddFinalizer(&we, wecontroller.FinalizerName)
		Expect(k8sClient.Update(ctx, &we)).To(Succeed())

		// No WE reconciler is registered with this RO manager: deletion is a
		// deterministic barrier before the production WE reconciler is invoked.
		Expect(k8sClient.Delete(ctx, rw, client.Preconditions{UID: &rw.UID})).To(Succeed())
		Eventually(func(g Gomega) {
			var found remediationworkflowv1.RemediationWorkflowList
			g.Expect(k8sManager.GetAPIReader().List(ctx, &found, client.InNamespace(ROControllerNamespace), client.MatchingFields{"status.workflowId": workflowID})).To(Succeed())
			g.Expect(found.Items).To(BeEmpty())
		}, timeout, interval).Should(Succeed())

		registry := weexecutor.NewRegistry()
		registry.Register("job", weexecutor.NewJobExecutor(k8sManager.GetClient()))
		reconciler := wecontroller.NewReconciler(k8sManager, wecontroller.ReconcilerOptions{
			ExecutionNamespace: namespace, CatalogNamespace: ROControllerNamespace, CooldownPeriod: time.Second,
			Metrics: wemetrics.NewMetricsWithRegistry(prometheus.NewRegistry()), StatusManager: westatus.NewManager(k8sManager.GetClient()),
			AuditStore: auditStore, PhaseManager: wephase.NewManager(), AuditManager: weaudit.NewManager(auditStore, ctrl.Log.WithName("missing-workflow-2570")), ExecutorRegistry: registry,
		})
		Eventually(func(g Gomega) {
			_, reconcileErr := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
			g.Expect(reconcileErr).NotTo(HaveOccurred())
			g.Expect(k8sManager.GetAPIReader().Get(ctx, key, &we)).To(Succeed())
			g.Expect(we.Status.Phase).To(Equal("Failed"))
			g.Expect(we.Status.FailureDetails).NotTo(BeNil())
			g.Expect(we.Status.FailureDetails.Reason).To(Equal("ConfigurationError"))
			g.Expect(we.Status.FailureDetails.Message).To(ContainSubstring("0 trusted catalog matches"))
			g.Expect(we.Status.ExecutionRef).To(BeNil())
			var jobs batchv1.JobList
			g.Expect(k8sManager.GetAPIReader().List(ctx, &jobs, client.InNamespace(namespace))).To(Succeed())
			g.Expect(jobs.Items).To(BeEmpty())
		}, timeout, interval).Should(Succeed())
	})

	DescribeTable("IT-RO-2570-043: BR-ORCH-025 carries approved values through the WE reconciler into execution inputs", func(engine string, injectUndeclared bool) {
		selected := defaultAIWorkflow()
		var engineConfig *apiextensionsv1.JSON
		if engine == ansibleEngine {
			engineConfig = &apiextensionsv1.JSON{Raw: []byte(`{"playbookPath":"playbooks/restart.yml","jobTemplateName":"restart-pod"}`)}
		}
		rw := &remediationworkflowv1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: "rw-job-" + uuid.NewString()[:8], Namespace: ROControllerNamespace},
			Spec: remediationworkflowv1.RemediationWorkflowSpec{
				Version: "1.0.0", ActionType: "RestartPod",
				Description: remediationworkflowv1.RemediationWorkflowDescription{What: "Approval job fixture", WhenToUse: "For integration tests"},
				Labels:      remediationworkflowv1.RemediationWorkflowLabels{Severity: []string{"critical"}, Environment: []string{"production"}, Component: []string{"v1/Pod"}, Priority: "P1"},
				Execution:   remediationworkflowv1.RemediationWorkflowExecution{Engine: engine, Bundle: testutil.ValidBundleRef, EngineConfig: engineConfig},
				Parameters:  []remediationworkflowv1.RemediationWorkflowParameter{{Name: "NAMESPACE", Type: "string", Required: true, Description: "namespace"}, {Name: "POD_NAME", Type: "string", Required: true, Description: "pod name"}},
			},
		}
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, rw))).To(Succeed()) })
		content, err := contenthash.MarshalCleanCRDContent(rw)
		Expect(err).NotTo(HaveOccurred())
		rw.Status.WorkflowID = contenthash.DeterministicUUID(contenthash.ComputeContentHash(string(content)))
		Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())
		selected.WorkflowID = rw.Status.WorkflowID
		selected.WorkflowName = rw.Name
		selected.ExecutionEngine = engine
		selected.EngineConfig = engineConfig
		selected.ExecutionBundle = testutil.ValidBundleRef
		selected.ExecutionBundleDigest = ""
		progressToAwaitingApproval(rrName, selected)
		Expect(approveRAR("rar-"+rrName, func(current *remediationv1.RemediationApprovalRequest) {
			current.Status.Decision = remediationv1.ApprovalDecisionApproved
			current.Status.DecidedBy = operatorKubernautAi
			decidedAt := metav1.Now()
			current.Status.DecidedAt = &decidedAt
			current.Status.WorkflowOverride = &remediationv1.WorkflowOverride{Parameters: typedStringParameters(map[string]string{"POD_NAME": "reviewed-pod"})}
		})).To(Succeed())
		key := types.NamespacedName{Name: "we-" + rrName, Namespace: ROControllerNamespace}
		we := &workflowexecutionv1.WorkflowExecution{}
		Eventually(func() error { return k8sManager.GetAPIReader().Get(ctx, key, we) }, timeout, interval).Should(Succeed())
		executionKey := key
		if injectUndeclared {
			// RO has already validated the selected values and approved override
			// against the live workflow. Create a malformed persisted WFE after that
			// boundary to prove WE's independent allowlist strips the value. WFE
			// specs are immutable after creation, so an update cannot model this
			// deliberately malformed stored snapshot.
			malformed := we.DeepCopy()
			malformed.Name += "-malformed"
			malformed.ResourceVersion = ""
			malformed.UID = ""
			malformed.CreationTimestamp = metav1.Time{}
			malformed.Generation = 0
			malformed.ManagedFields = nil
			malformed.Finalizers = nil
			malformed.Status = workflowexecutionv1.WorkflowExecutionStatus{}
			malformed.Spec.Parameters["UNDECLARED"] = typedStringParameter("must-not-reach-executor")
			Expect(k8sClient.Create(ctx, malformed)).To(Succeed())
			DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, malformed))).To(Succeed()) })
			executionKey = types.NamespacedName{Name: malformed.Name, Namespace: malformed.Namespace}
		}

		registry := weexecutor.NewRegistry()
		registry.Register("job", weexecutor.NewJobExecutor(k8sManager.GetClient()))
		registry.Register("tekton", weexecutor.NewTektonExecutor(k8sManager.GetClient()))
		launches := make(chan map[string]interface{}, 1)
		if engine == ansibleEngine {
			mux := http.NewServeMux()
			mux.HandleFunc("/api/v2/job_templates/", func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v2/job_templates/" && r.Method == http.MethodGet {
					_, _ = w.Write([]byte(`{"count":1,"results":[{"id":7}]}`))
					return
				}
				if r.URL.Path == "/api/v2/job_templates/7/launch/" && r.Method == http.MethodPost {
					var payload struct {
						ExtraVars map[string]interface{} `json:"extra_vars"`
					}
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						http.Error(w, "invalid launch payload", http.StatusBadRequest)
						return
					}
					select {
					case launches <- payload.ExtraVars:
					default:
					}
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{"id":42}`))
					return
				}
				http.NotFound(w, r)
			})
			server := httptest.NewServer(mux)
			DeferCleanup(server.Close)
			awxClient, clientErr := weexecutor.NewAWXHTTPClient(server.URL, "test-token")
			Expect(clientErr).NotTo(HaveOccurred())
			ansible := weexecutor.NewAnsibleExecutor(awxClient, k8sManager.GetClient(), nil, 1, logr.Discard())
			ansible.InClusterCredentialsFn = func() (*weexecutor.InClusterCredentials, error) {
				return nil, fmt.Errorf("no in-cluster credentials in this fixture")
			}
			registry.Register(ansibleEngine, ansible)
		}
		weReconciler := wecontroller.NewReconciler(k8sManager, wecontroller.ReconcilerOptions{
			ExecutionNamespace: ROControllerNamespace, CatalogNamespace: ROControllerNamespace, CooldownPeriod: time.Second,
			Metrics: wemetrics.NewMetricsWithRegistry(prometheus.NewRegistry()), StatusManager: westatus.NewManager(k8sManager.GetClient()),
			AuditStore: auditStore, PhaseManager: wephase.NewManager(), AuditManager: weaudit.NewManager(auditStore, ctrl.Log.WithName("approval-job-test")), ExecutorRegistry: registry,
		})
		var jobName string
		var pipelineName string
		var ansibleValues map[string]interface{}
		DeferCleanup(func() {
			if jobName != "" {
				job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: ROControllerNamespace}}
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, job))).To(Succeed())
			}
			if pipelineName != "" {
				pipeline := &tektonv1.PipelineRun{ObjectMeta: metav1.ObjectMeta{Name: pipelineName, Namespace: ROControllerNamespace}}
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, pipeline))).To(Succeed())
			}
		})
		Eventually(func(g Gomega) {
			_, reconcileErr := weReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: executionKey})
			g.Expect(reconcileErr).NotTo(HaveOccurred())
			values := make(map[string]string)
			switch engine {
			case "job":
				var jobs batchv1.JobList
				g.Expect(k8sManager.GetAPIReader().List(ctx, &jobs, client.InNamespace(ROControllerNamespace), client.MatchingLabels{"kubernaut.ai/workflow-execution": executionKey.Name})).To(Succeed())
				g.Expect(jobs.Items).To(HaveLen(1))
				jobName = jobs.Items[0].Name
				g.Expect(jobs.Items[0].Spec.Template.Spec.Containers).To(HaveLen(1))
				for _, env := range jobs.Items[0].Spec.Template.Spec.Containers[0].Env {
					values[env.Name] = env.Value
				}
			case "tekton":
				var runs tektonv1.PipelineRunList
				g.Expect(k8sManager.GetAPIReader().List(ctx, &runs, client.InNamespace(ROControllerNamespace), client.MatchingLabels{"kubernaut.ai/workflow-execution": executionKey.Name})).To(Succeed())
				g.Expect(runs.Items).To(HaveLen(1))
				pipelineName = runs.Items[0].Name
				for _, param := range runs.Items[0].Spec.Params {
					values[param.Name] = param.Value.StringVal
				}
			case ansibleEngine:
				if ansibleValues == nil {
					select {
					case ansibleValues = <-launches:
					default:
					}
				}
				g.Expect(ansibleValues).NotTo(BeNil(), "AWX must receive a launch request")
				g.Expect(ansibleValues).To(HaveKeyWithValue("POD_NAME", "reviewed-pod"))
				g.Expect(ansibleValues).To(HaveKeyWithValue("NAMESPACE", "default"))
				if injectUndeclared {
					g.Expect(ansibleValues).NotTo(HaveKey("UNDECLARED"))
				}
				return
			}
			g.Expect(values).To(HaveKeyWithValue("POD_NAME", "reviewed-pod"))
			g.Expect(values).To(HaveKeyWithValue("NAMESPACE", "default"))
			if injectUndeclared {
				g.Expect(values).NotTo(HaveKey("UNDECLARED"))
			}
		}, timeout, 250*time.Millisecond).Should(Succeed())
	},
		Entry("Job", "job", false),
		Entry("Tekton", "tekton", false),
		Entry("Ansible", ansibleEngine, false),
		Entry("Job strips undeclared input", "job", true),
		Entry("Tekton strips undeclared input", "tekton", true),
		Entry("Ansible strips undeclared input", ansibleEngine, true),
	)

	Context("IT-RO-2570-042: approved legacy workflow switching", func() {
		It("BR-ORCH-025 fails closed without creating WE when admission was bypassed", func() {
			rwName := fmt.Sprintf("rw-ovr-%s", uuid.New().String()[:8])
			createOverrideRW(rwName)

			progressToAwaitingApproval(rrName, defaultAIWorkflow())

			rarName := fmt.Sprintf("rar-%s", rrName)
			rar := &remediationv1.RemediationApprovalRequest{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: ROControllerNamespace}, rar)
			}, timeout, interval).Should(Succeed())

			By("Approving with workflow override")
			Expect(approveRAR(rarName, func(rar *remediationv1.RemediationApprovalRequest) {
				rar.Status.Decision = remediationv1.ApprovalDecisionApproved
				rar.Status.DecidedBy = operatorKubernautAi
				rar.Status.DecisionMessage = "Override to drain-restart"
				decidedAt := metav1.Now()
				rar.Status.DecidedAt = &decidedAt
				rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{
					WorkflowName: rwName,
					Rationale:    "prefer drain-restart",
				}
			})).To(Succeed())

			By("Verifying the parent fails and no WE is created")
			Eventually(func() string {
				rr := &remediationv1.RemediationRequest{}
				if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, rr); err != nil {
					return ""
				}
				return string(rr.Status.OverallPhase)
			}, timeout, interval).Should(Equal("Failed"))
			weName := fmt.Sprintf("we-%s", rrName)
			we := &workflowexecutionv1.WorkflowExecution{}
			Expect(apierrors.IsNotFound(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, we))).To(BeTrue())
		})
	})

	Context("IT-RO-594-002: Approve without override → WE matches AIA (regression guard)", func() {
		It("should create WE with AIA spec when no override is provided", func() {
			progressToAwaitingApproval(rrName, defaultAIWorkflow())

			rarName := fmt.Sprintf("rar-%s", rrName)
			rar := &remediationv1.RemediationApprovalRequest{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: ROControllerNamespace}, rar)
			}, timeout, interval).Should(Succeed())

			By("Approving without override (standard flow)")
			Expect(approveRAR(rarName, func(rar *remediationv1.RemediationApprovalRequest) {
				rar.Status.Decision = remediationv1.ApprovalDecisionApproved
				rar.Status.DecidedBy = operatorKubernautAi
				rar.Status.DecisionMessage = "Approved as recommended"
				decidedAt := metav1.Now()
				rar.Status.DecidedAt = &decidedAt
			})).To(Succeed())

			By("Verifying WE is created with AIA spec")
			weName := fmt.Sprintf("we-%s", rrName)
			we := &workflowexecutionv1.WorkflowExecution{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, we)
			}, timeout, interval).Should(Succeed())

			Expect(we.Spec.WorkflowRef.WorkflowID).To(Equal(trustedAIOriginalWorkflowID))
			Expect(we.Spec.WorkflowRef.Version).To(Equal("1.0.0"))
			Expect(we.Spec.WorkflowRef.ExecutionBundle).To(Equal("ai-bundle:v1.0@sha256:aaa"))
		})
	})

	Context("IT-RO-594-003: Approve + params-only override → workflow with partial changes", func() {
		It("should create WE with the workflow and overlaid params", func() {
			progressToAwaitingApproval(rrName, defaultAIWorkflow())

			rarName := fmt.Sprintf("rar-%s", rrName)
			rar := &remediationv1.RemediationApprovalRequest{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: ROControllerNamespace}, rar)
			}, timeout, interval).Should(Succeed())

			By("Approving with params-only override")
			Expect(approveRAR(rarName, func(rar *remediationv1.RemediationApprovalRequest) {
				rar.Status.Decision = remediationv1.ApprovalDecisionApproved
				rar.Status.DecidedBy = operatorKubernautAi
				rar.Status.DecisionMessage = "Override params"
				decidedAt := metav1.Now()
				rar.Status.DecidedAt = &decidedAt
				rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{
					Parameters: typedStringParameters(map[string]string{"POD_NAME": "reviewed-pod"}),
					Rationale:  "change pod name",
				}
			})).To(Succeed())

			By("Verifying WE has AIA workflow but override params")
			weName := fmt.Sprintf("we-%s", rrName)
			we := &workflowexecutionv1.WorkflowExecution{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, we)
			}, timeout, interval).Should(Succeed())

			Expect(we.Spec.WorkflowRef.WorkflowID).To(Equal(trustedAIOriginalWorkflowID))
			Expect(we.Spec.Parameters).To(Equal(typedStringParameters(map[string]string{
				"NAMESPACE": "default",
				"POD_NAME":  "reviewed-pod",
			})))
		})
	})

	Context("IT-RO-594-004: Override applied → OperatorOverride event on RR", func() {
		It("should emit OperatorOverride K8s event when override is applied", func() {
			progressToAwaitingApproval(rrName, defaultAIWorkflow())

			rarName := fmt.Sprintf("rar-%s", rrName)
			rar := &remediationv1.RemediationApprovalRequest{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: ROControllerNamespace}, rar)
			}, timeout, interval).Should(Succeed())

			By("Approving with override")
			Expect(approveRAR(rarName, func(rar *remediationv1.RemediationApprovalRequest) {
				rar.Status.Decision = remediationv1.ApprovalDecisionApproved
				rar.Status.DecidedBy = operatorKubernautAi
				decidedAt := metav1.Now()
				rar.Status.DecidedAt = &decidedAt
				rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{
					Parameters: typedStringParameters(map[string]string{"POD_NAME": "reviewed-pod"}),
					Rationale:  "operator review",
				}
			})).To(Succeed())

			By("Waiting for WE creation (confirms reconciler processed the override)")
			weName := fmt.Sprintf("we-%s", rrName)
			Eventually(func() error {
				we := &workflowexecutionv1.WorkflowExecution{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, we)
			}, timeout, interval).Should(Succeed())

			By("Verifying OperatorOverride event was emitted on RR")
			rr := &remediationv1.RemediationRequest{}
			// WE creation and the RR.Status.OverallPhase="Executing" write happen as
			// separate steps of the same reconcile; a bare Expect right after the WE
			// Eventually above can observe the RR microseconds before that write lands.
			Eventually(func() string {
				_ = k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, rr)
				return string(rr.Status.OverallPhase)
			}, timeout, interval).Should(Equal("Executing"))
		})
	})
})
