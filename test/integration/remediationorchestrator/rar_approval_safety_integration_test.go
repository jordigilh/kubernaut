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
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	gwhelpers "github.com/jordigilh/kubernaut/test/integration/gateway/helpers"
	"github.com/jordigilh/kubernaut/test/shared/integration"
)

var _ = Describe("RAR approval safety production wiring", Label("integration", "rar", "approval-safety"), func() {
	It("IT-RO-2570-026: BR-ORCH-025 routes a missing approval to manual review without replacement or execution", func() {
		rr := createRemediationRequest(ROControllerNamespace, "rr-rar-missing-"+uuid.NewString()[:8])
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })
		setRARSafetyParentPhase(rr.Name, remediationv1.PhaseAwaitingApproval)
		current := &remediationv1.RemediationRequest{}
		Eventually(func() remediationv1.RemediationPhase {
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), current); err != nil {
				return ""
			}
			return current.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))
		Expect(current.Status.CompletionStatus).NotTo(BeNil())
		Expect(current.Status.CompletionStatus.FailurePhase).NotTo(BeNil())
		Expect(*current.Status.CompletionStatus.FailurePhase).To(Equal(remediationv1.FailurePhaseApproval))
		Expect(current.Status.CompletionStatus.RequiresManualReview).To(BeTrue())
		Expect(current.Status.CompletionStatus.Outcome).To(Equal(remediationv1.OutcomeManualReviewRequired))
		rar := &remediationv1.RemediationApprovalRequest{}
		Expect(apierrors.IsNotFound(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: "rar-" + rr.Name, Namespace: rr.Namespace}, rar))).To(BeTrue())
		wes := &workflowexecutionv1.WorkflowExecutionList{}
		Expect(k8sManager.GetAPIReader().List(ctx, wes, client.InNamespace(rr.Namespace))).To(Succeed())
		for _, we := range wes.Items {
			for _, owner := range we.OwnerReferences {
				Expect(owner.UID).NotTo(Equal(rr.UID))
			}
		}
	})
	It("IT-AUD-2570-044: BR-AUDIT-006 records enqueue failure without blocking the committed decision", func() {
		namespace := createTestNamespace(ctx, "rar-audit-enqueue")
		DeferCleanup(func() { deleteTestNamespace(namespace) })
		direct, err := client.New(cfg, client.Options{Scheme: k8sManager.GetScheme()})
		Expect(err).NotTo(HaveOccurred())
		rar := rarSafetyRequest(nil, time.Now().Add(15*time.Minute), false)
		rar.Namespace = namespace
		rar.Spec.RemediationRequestRef.Namespace = namespace
		Expect(direct.Create(ctx, rar)).To(Succeed())
		rar.Status.Decision = remediationv1.ApprovalDecisionApproved
		rar.Status.DecidedBy = "reviewer@example.com"
		rar.Status.DecidedAt = &metav1.Time{Time: time.Now()}
		Expect(direct.Status().Update(ctx, rar)).To(Succeed())

		store, err := audit.NewBufferedStore(dsClients.AuditClient, audit.DefaultConfig(), "rar-enqueue-failure-test", logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(store.Close()).To(Succeed())
		reconciler := controller.NewRARReconciler(direct, direct, k8sManager.GetScheme(), store, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
		_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(rar)})
		Expect(err).NotTo(HaveOccurred(), "audit enqueue failure must not block the committed decision")
		persisted := &remediationv1.RemediationApprovalRequest{}
		Expect(direct.Get(ctx, client.ObjectKeyFromObject(rar), persisted)).To(Succeed())
		Expect(persisted.Status.Decision).To(Equal(remediationv1.ApprovalDecisionApproved))
		condition := meta.FindStatusCondition(persisted.Status.Conditions, remediationapprovalrequest.ConditionAuditRecorded)
		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.Reason).To(Equal(remediationapprovalrequest.ReasonAuditFailed))
		Expect(condition.Message).NotTo(ContainSubstring("accepted by audit buffer"))
	})
	It("IT-AUD-2570-007: BR-AUDIT-005 reads the committed approval values from DataStorage", func() {
		rar := rarSafetyRequest(nil, time.Now().Add(15*time.Minute), false)
		rar.Spec.RemediationRequestRef.Name = "rr-audit-2570-" + uuid.NewString()[:8]
		rar.Spec.Workflow = &sharedtypes.WorkflowSnapshot{WorkflowID: "wf-reviewed", WorkflowName: "wf-reviewed", ActionType: "RestartPod", Version: "v1", ExecutionBundle: "example:v1", DeclaredParameterNames: map[string]bool{"P": true, "Q": true}}
		rar.Spec.Parameters = map[string]string{"P": "2", "Q": "keep"}
		rar.Spec.ParameterSchema = []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "float", Description: "threshold"}, {Name: "Q", Type: "string", Description: "unchanged"}}
		rar.Spec.TargetContext = &remediationv1.ApprovalTargetContext{SignalTarget: remediationv1.ResourceIdentifier{Kind: "Pod", Name: "pod"}}
		Expect(k8sClient.Create(ctx, rar)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, rar))).To(Succeed()) })
		rar.Status.Decision = remediationv1.ApprovalDecisionApproved
		rar.Status.DecidedBy = "reviewer@example.com"
		rar.Status.DecidedAt = &metav1.Time{Time: time.Now()}
		rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{Parameters: map[string]string{"P": "1.0"}, Rationale: "reviewed exactly"}
		Expect(k8sClient.Status().Update(ctx, rar)).To(Succeed())
		var events []ogenclient.AuditEvent
		Eventually(func() int {
			if err := auditStore.Flush(ctx); err != nil {
				GinkgoWriter.Printf("audit buffer flush: %v\n", err)
			}
			events = queryAuditEventsOpenAPI(dsClients.OpenAPIClient, rar.Spec.RemediationRequestRef.Name, "orchestrator.approval.approved")
			return len(events)
		}, 30*time.Second, 500*time.Millisecond).Should(Equal(1))
		payload := events[0].EventData.RemediationOrchestratorAuditPayload
		original, hasOriginal := payload.OriginalParameters.Get()
		changes, hasChanges := payload.OverrideParameters.Get()
		effective, hasEffective := payload.EffectiveParameters.Get()
		Expect(hasOriginal).To(BeTrue())
		Expect(hasChanges).To(BeTrue())
		Expect(hasEffective).To(BeTrue())
		Expect(original).To(HaveKeyWithValue("P", "2"))
		Expect(changes).To(HaveKeyWithValue("P", "1.0"))
		Expect(effective).To(HaveKeyWithValue("P", "1.0"))
		Expect(effective).To(HaveKeyWithValue("Q", "keep"))
		Expect(payload.WorkflowID.Value).To(Equal("wf-reviewed"))
		Expect(payload.OverrideRationale.Value).To(Equal("reviewed exactly"))
		Expect(events[0].ActorID.Value).To(Equal("reviewer@example.com"))

		By("Denying an authenticated service account without audit-read authorization")
		clientset, err := kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		saHelper := gwhelpers.NewServiceAccountHelper(clientset, k8sClient, ROControllerNamespace)
		saName := "audit-denied-" + uuid.NewString()[:8]
		Expect(saHelper.CreateServiceAccount(ctx, saName)).To(Succeed())
		DeferCleanup(func() { Expect(saHelper.DeleteServiceAccount(ctx, saName)).To(Succeed()) })
		token, err := saHelper.GetServiceAccountToken(ctx, saName)
		Expect(err).NotTo(HaveOccurred())
		deniedClient := integration.NewAuthenticatedDataStorageClients(dataStorageBaseURL, token, 10*time.Second)
		query := url.Values{"correlation_id": {rar.Spec.RemediationRequestRef.Name}, "event_category": {"orchestration"}, "event_type": {"orchestrator.approval.approved"}}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, dataStorageBaseURL+"/api/v1/audit/events?"+query.Encode(), nil)
		Expect(err).NotTo(HaveOccurred())
		response, err := deniedClient.HTTPClient.Do(request)
		Expect(err).NotTo(HaveOccurred())
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusForbidden), "a valid but unauthorized token must be rejected by authorization, not authentication")
		Expect(string(body)).NotTo(ContainSubstring("reviewed exactly"))
	})
	It("IT-RO-2570-025: BR-ORCH-025 does not create an approval request without a parameter schema", func() {
		rrName := fmt.Sprintf("rr-rar-no-schema-%s", uuid.NewString()[:8])
		rr := createRemediationRequest(ROControllerNamespace, rrName)
		name := "rar-" + rrName
		DeferCleanup(func() {
			rarSafetyRemoveFinalizer(name)
			_ = k8sClient.Delete(ctx, &remediationv1.RemediationApprovalRequest{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ROControllerNamespace}})
			_ = k8sClient.Delete(ctx, rr)
		})
		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "ai-" + rrName, Namespace: ROControllerNamespace},
			Status: aianalysisv1.AIAnalysisStatus{
				Approval: &aianalysisv1.ApprovalStatus{ApprovalRequired: true, ApprovalReason: "low confidence"},
				RCAResult: &aianalysisv1.RCAResult{SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowSnapshot: sharedtypes.WorkflowSnapshot{WorkflowID: "wf-schema", WorkflowName: "wf-schema", ActionType: "RestartPod", Version: "v1", ExecutionBundle: "oci://example.invalid/workflows:test", DeclaredParameterNames: map[string]bool{}},
					Confidence:       0.4, Rationale: "schema fixture", Parameters: map[string]string{},
				}},
			},
		}
		approvalCreator := creator.NewApprovalCreator(k8sClient, k8sManager.GetScheme(), nil, 15*time.Minute)
		_, err := approvalCreator.Create(ctx, rr, ai)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("schema"))
		persisted := &remediationv1.RemediationApprovalRequest{}
		err = k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: name, Namespace: ROControllerNamespace}, persisted)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("IT-RO-2570-024: BR-ORCH-025 persists the selected parameter schema through the API server", func() {
		rrName := fmt.Sprintf("rr-rar-schema-%s", uuid.NewString()[:8])
		rr := createRemediationRequest(ROControllerNamespace, rrName)
		name := "rar-" + rrName
		DeferCleanup(func() {
			rarSafetyRemoveFinalizer(name)
			_ = k8sClient.Delete(ctx, &remediationv1.RemediationApprovalRequest{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ROControllerNamespace}})
			_ = k8sClient.Delete(ctx, rr)
		})
		selected := &aianalysisv1.SelectedWorkflow{}
		Expect(json.Unmarshal([]byte(`{"workflowId":"wf-schema","workflowName":"wf-schema","actionType":"RestartPod","version":"v1","executionBundle":"oci://example.invalid/workflows:test","declaredParameterNames":{"REPLICAS":true},"parameters":{"REPLICAS":"3"},"confidence":0.4,"rationale":"schema fixture","parameterSchema":[{"name":"REPLICAS","type":"integer","required":true,"description":"count","minimum":1,"maximum":9,"default":3}]}`), selected)).To(Succeed())
		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "ai-" + rrName, Namespace: ROControllerNamespace},
			Status: aianalysisv1.AIAnalysisStatus{
				Approval:  &aianalysisv1.ApprovalStatus{ApprovalRequired: true, ApprovalReason: "low confidence"},
				RCAResult: &aianalysisv1.RCAResult{SelectedWorkflow: selected},
			},
		}
		approvalCreator := creator.NewApprovalCreator(k8sClient, k8sManager.GetScheme(), nil, 15*time.Minute)
		_, err := approvalCreator.Create(ctx, rr, ai)
		Expect(err).NotTo(HaveOccurred())
		persisted := &remediationv1.RemediationApprovalRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: name, Namespace: ROControllerNamespace}, persisted)).To(Succeed())
		raw, err := json.Marshal(persisted.Spec)
		Expect(err).NotTo(HaveOccurred())
		var spec map[string]json.RawMessage
		Expect(json.Unmarshal(raw, &spec)).To(Succeed())
		Expect(spec).To(HaveKey("parameterSchema"))
		Expect(string(spec["parameterSchema"])).To(MatchJSON(`[{"name":"REPLICAS","type":"integer","required":true,"description":"count","minimum":1,"maximum":9,"default":3}]`))
	})

	It("IT-2570-021: BR-ORCH-025 fails an approved legacy RAR without a snapshot", func() {
		rrName := fmt.Sprintf("rr-rar-old-%s", uuid.NewString()[:8])
		rr := createRemediationRequest(ROControllerNamespace, rrName)
		rar := rarSafetyRequest(rr, time.Now().Add(15*time.Minute), true)
		Expect(k8sClient.Create(ctx, rar)).To(Succeed())
		DeferCleanup(func() {
			rarSafetyRemoveFinalizer(rar.Name)
			_ = k8sClient.Delete(ctx, rar)
			_ = k8sClient.Delete(ctx, rr)
		})
		Expect(approveRAR(rar.Name, func(current *remediationv1.RemediationApprovalRequest) {
			current.Status.Decision = remediationv1.ApprovalDecisionApproved
			current.Status.DecidedBy = "operator@example.com"
			decidedAt := metav1.Now()
			current.Status.DecidedAt = &decidedAt
		})).To(Succeed())
		setRARSafetyParentPhase(rr.Name, remediationv1.PhaseAwaitingApproval)
		Eventually(func() remediationv1.RemediationPhase {
			current := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), current); err != nil {
				return ""
			}
			return current.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))
		we := &workflowexecutionv1.WorkflowExecution{}
		err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: "we-" + rrName, Namespace: ROControllerNamespace}, we)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
	It("IT-2570-021: BR-ORCH-025 does not create an approval request without declaration metadata", func() {
		rrName := fmt.Sprintf("rr-rar-missing-declarations-%s", uuid.NewString()[:8])
		rr := createRemediationRequest(ROControllerNamespace, rrName)
		name := "rar-" + rrName
		DeferCleanup(func() {
			rarSafetyRemoveFinalizer(name)
			_ = k8sClient.Delete(ctx, &remediationv1.RemediationApprovalRequest{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ROControllerNamespace}})
			_ = k8sClient.Delete(ctx, rr)
		})
		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "ai-" + rrName, Namespace: ROControllerNamespace},
			Status: aianalysisv1.AIAnalysisStatus{
				Approval: &aianalysisv1.ApprovalStatus{ApprovalRequired: true, ApprovalReason: "low confidence"},
				RCAResult: &aianalysisv1.RCAResult{SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowSnapshot: sharedtypes.WorkflowSnapshot{WorkflowID: "wf-declarations", WorkflowName: "wf-declarations", ActionType: "RestartPod", Version: "v1", ExecutionBundle: "oci://example.invalid/workflows:test"},
					Confidence:       0.4, Rationale: "declaration fixture", Parameters: map[string]string{},
				}},
			},
		}
		approvalCreator := creator.NewApprovalCreator(k8sClient, k8sManager.GetScheme(), nil, 15*time.Minute)
		_, err := approvalCreator.Create(ctx, rr, ai)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("declar"))
		persisted := &remediationv1.RemediationApprovalRequest{}
		err = k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: name, Namespace: ROControllerNamespace}, persisted)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	DescribeTable("IT-RO-2570-002: BR-ORCH-026 preserves absent and invalid remediation targets through the API server", func(target *aianalysisv1.RemediationTarget) {
		rrName := fmt.Sprintf("rr-rar-target-%s", uuid.NewString()[:8])
		rr := createRemediationRequest(ROControllerNamespace, rrName)
		name := "rar-" + rrName
		DeferCleanup(func() {
			rarSafetyRemoveFinalizer(name)
			_ = k8sClient.Delete(ctx, &remediationv1.RemediationApprovalRequest{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ROControllerNamespace}})
			_ = k8sClient.Delete(ctx, rr)
		})
		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "ai-" + rrName, Namespace: ROControllerNamespace},
			Status: aianalysisv1.AIAnalysisStatus{
				Approval: &aianalysisv1.ApprovalStatus{ApprovalRequired: true, ApprovalReason: "low confidence"},
				RCAResult: &aianalysisv1.RCAResult{
					RootCauseAnalysis: &aianalysisv1.RootCauseAnalysis{RemediationTarget: target},
					SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
						WorkflowSnapshot: sharedtypes.WorkflowSnapshot{WorkflowID: "wf-target", WorkflowName: "wf-target", ActionType: "RestartPod", Version: "v1", ExecutionBundle: "oci://example.invalid/workflows:test", DeclaredParameterNames: map[string]bool{}},
						Confidence:       0.4, Rationale: "target fixture", Parameters: map[string]string{}, ParameterSchema: []remediationworkflowv1.RemediationWorkflowParameter{},
					},
				},
			},
		}
		approvalCreator := creator.NewApprovalCreator(k8sClient, k8sManager.GetScheme(), nil, 15*time.Minute)
		_, err := approvalCreator.Create(ctx, rr, ai)
		Expect(err).NotTo(HaveOccurred())
		persisted := &remediationv1.RemediationApprovalRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: name, Namespace: ROControllerNamespace}, persisted)).To(Succeed())
		Expect(persisted.Spec.TargetContext).NotTo(BeNil())
		if target == nil {
			Expect(persisted.Spec.TargetContext.RemediationTarget).To(BeNil())
		} else {
			Expect(persisted.Spec.TargetContext.RemediationTarget).NotTo(BeNil())
			Expect(persisted.Spec.TargetContext.RemediationTarget.Kind).To(Equal(target.Kind))
			Expect(persisted.Spec.TargetContext.RemediationTarget.Name).To(Equal(target.Name))
		}
	},
		Entry("absent", (*aianalysisv1.RemediationTarget)(nil)),
		Entry("invalid", &aianalysisv1.RemediationTarget{Kind: "Deployment", Namespace: "default"}),
	)

	It("IT-RO-2570-002: BR-ORCH-025 persists the selected workflow and parameters in the approval request", func() {
		rrName := fmt.Sprintf("rr-rar-snapshot-%s", uuid.NewString()[:8])
		rr := createRemediationRequest(ROControllerNamespace, rrName)
		rr.Spec.ClusterID = "cluster-a"
		rr.Spec.TargetResource = remediationv1.ResourceIdentifier{APIVersion: "v1", Kind: "Pod", Namespace: ROControllerNamespace, Name: "pod-a"}
		name := "rar-" + rrName
		DeferCleanup(func() {
			rarSafetyRemoveFinalizer(name)
			_ = k8sClient.Delete(ctx, &remediationv1.RemediationApprovalRequest{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ROControllerNamespace}})
			_ = k8sClient.Delete(ctx, rr)
		})
		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "ai-" + rrName, Namespace: ROControllerNamespace},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:    aianalysisv1.PhaseCompleted,
				Approval: &aianalysisv1.ApprovalStatus{ApprovalRequired: true, ApprovalReason: "low confidence"},
				RCAResult: &aianalysisv1.RCAResult{RootCauseAnalysis: &aianalysisv1.RootCauseAnalysis{RemediationTarget: &aianalysisv1.RemediationTarget{APIVersion: "apps/v1", Kind: "Deployment", Namespace: ROControllerNamespace, Name: "app-a"}}, SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowSnapshot: sharedtypes.WorkflowSnapshot{WorkflowID: "wf-snapshot", WorkflowName: "wf-snapshot", ActionType: "RestartPod", Version: "v1", ExecutionBundle: "oci://example.invalid/workflows:test", DeclaredParameterNames: map[string]bool{"REPLICAS": true, "MODE": true}},
					Confidence:       0.4, Rationale: "snapshot fixture", Parameters: map[string]string{"REPLICAS": "3", "MODE": "safe"}, ParameterSchema: []remediationworkflowv1.RemediationWorkflowParameter{{Name: "REPLICAS", Type: "integer", Required: true, Description: "replicas"}, {Name: "MODE", Type: "string", Required: true, Description: "mode"}},
				}},
			},
		}
		approvalCreator := creator.NewApprovalCreator(k8sClient, k8sManager.GetScheme(), nil, 15*time.Minute)
		_, err := approvalCreator.Create(ctx, rr, ai)
		Expect(err).NotTo(HaveOccurred())
		persisted := &remediationv1.RemediationApprovalRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: name, Namespace: ROControllerNamespace}, persisted)).To(Succeed())
		raw, err := json.Marshal(persisted.Spec)
		Expect(err).NotTo(HaveOccurred())
		var spec map[string]json.RawMessage
		Expect(json.Unmarshal(raw, &spec)).To(Succeed())
		Expect(spec).To(HaveKey("workflow"))
		Expect(spec).To(HaveKey("parameters"))
		var workflow struct {
			WorkflowID string `json:"workflowId"`
		}
		var params map[string]string
		Expect(json.Unmarshal(spec["workflow"], &workflow)).To(Succeed())
		Expect(json.Unmarshal(spec["parameters"], &params)).To(Succeed())
		Expect(workflow.WorkflowID).To(Equal("wf-snapshot"))
		Expect(params).To(Equal(map[string]string{"REPLICAS": "3", "MODE": "safe"}))
		Expect(spec).To(HaveKey("targetContext"))
		var target struct {
			RemediationTarget *remediationv1.ResourceIdentifier `json:"remediationTarget"`
			SignalTarget      remediationv1.ResourceIdentifier  `json:"signalTarget"`
			SignalClusterID   string                            `json:"signalClusterId"`
		}
		Expect(json.Unmarshal(spec["targetContext"], &target)).To(Succeed())
		Expect(target.RemediationTarget).NotTo(BeNil())
		Expect(target.RemediationTarget.Name).To(Equal("app-a"))
		Expect(target.SignalTarget.Name).To(Equal("pod-a"))
		Expect(target.SignalClusterID).To(Equal("cluster-a"))
	})

	It("IT-RO-2570-002: BR-ORCH-025 does not create an approval request without original parameter data", func() {
		rrName := fmt.Sprintf("rr-rar-missing-params-%s", uuid.NewString()[:8])
		rr := createRemediationRequest(ROControllerNamespace, rrName)
		DeferCleanup(func() {
			rarSafetyRemoveFinalizer("rar-" + rrName)
			_ = k8sClient.Delete(ctx, &remediationv1.RemediationApprovalRequest{ObjectMeta: metav1.ObjectMeta{Name: "rar-" + rrName, Namespace: ROControllerNamespace}})
			_ = k8sClient.Delete(ctx, rr)
		})
		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "ai-" + rrName, Namespace: ROControllerNamespace},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:    aianalysisv1.PhaseCompleted,
				Approval: &aianalysisv1.ApprovalStatus{ApprovalRequired: true, ApprovalReason: "low confidence"},
				RCAResult: &aianalysisv1.RCAResult{SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowSnapshot: sharedtypes.WorkflowSnapshot{WorkflowID: "wf-missing-params", WorkflowName: "wf-missing-params", ActionType: "RestartPod", Version: "v1", ExecutionBundle: "oci://example.invalid/workflows:test"},
					Confidence:       0.4,
					Rationale:        "integration missing parameters fixture",
				}},
			},
		}
		approvalCreator := creator.NewApprovalCreator(k8sClient, k8sManager.GetScheme(), nil, 15*time.Minute)
		_, err := approvalCreator.Create(ctx, rr, ai)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("parameter"), "an unrelated API or fixture error must not satisfy the missing-parameters contract")
		persisted := &remediationv1.RemediationApprovalRequest{}
		err = k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: "rar-" + rrName, Namespace: ROControllerNamespace}, persisted)
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "missing data must not produce a reviewable RAR")
	})

	DescribeTable("IT-RO-2564-002: applies the RAR deletion matrix before releasing the finalizer",
		func(parentPhase remediationv1.RemediationPhase, shouldFailParent bool) {
			rrName := fmt.Sprintf("rr-rar-delete-%s", uuid.NewString()[:13])
			rr := createRemediationRequest(ROControllerNamespace, rrName)
			rar := rarSafetyRequest(rr, time.Now().Add(15*time.Minute), true)
			Expect(k8sClient.Create(ctx, rar)).To(Succeed())
			setRARSafetyParentPhase(rr.Name, parentPhase)
			DeferCleanup(func() {
				rarSafetyRemoveFinalizer(rar.Name)
				_ = k8sClient.Delete(ctx, rr)
			})

			Expect(k8sClient.Delete(ctx, rar)).To(Succeed())

			Eventually(func() bool {
				current := &remediationv1.RemediationApprovalRequest{}
				err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rar), current)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue(), "RAR finalizer should be released after the parent decision")

			currentRR := &remediationv1.RemediationRequest{}
			Eventually(func() remediationv1.RemediationPhase {
				if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), currentRR); err != nil {
					return ""
				}
				return currentRR.Status.OverallPhase
			}, timeout, interval).Should(Equal(func() remediationv1.RemediationPhase {
				if shouldFailParent {
					return remediationv1.PhaseFailed
				}
				return parentPhase
			}()))
		},
		Entry("awaiting approval deletion fails the parent", remediationv1.PhaseAwaitingApproval, true),
		Entry("terminal parent deletion does not mutate the parent", remediationv1.PhaseCompleted, false),
	)

	It("IT-RO-2564-003: parent deletion performs a direct terminating-RAR lookup", func() {
		rrName := fmt.Sprintf("rr-rar-orphan-%s", uuid.NewString()[:13])
		rr := createRemediationRequest(ROControllerNamespace, rrName)
		rar := rarSafetyRequest(rr, time.Now().Add(15*time.Minute), true)
		Expect(k8sClient.Create(ctx, rar)).To(Succeed())
		setRARSafetyParentPhase(rr.Name, remediationv1.PhaseAwaitingApproval)
		DeferCleanup(func() {
			rarSafetyRemoveFinalizer(rar.Name)
			_ = k8sClient.Delete(ctx, rr)
		})

		// Delete the parent first. Envtest does not run the Kubernetes
		// garbage-collector controller, so the RAR remains available with its
		// finalizer and the explicit RAR delete below exercises RO's NotFound
		// path without relying on GC fan-out.
		Expect(k8sClient.Delete(ctx, rr)).To(Succeed())
		Eventually(func() bool {
			current := &remediationv1.RemediationRequest{}
			return apierrors.IsNotFound(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), current))
		}, timeout, interval).Should(BeTrue())

		Expect(k8sClient.Delete(ctx, rar)).To(Succeed())
		Eventually(func() bool {
			current := &remediationv1.RemediationApprovalRequest{}
			return apierrors.IsNotFound(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rar), current))
		}, timeout, interval).Should(BeTrue(), "orphaned terminating RAR should be finalized after the parent is confirmed gone")
	})

	It("IT-RO-2564-007: production dispatch atomically confirms expiry before failing the parent", func() {
		rrName := fmt.Sprintf("rr-rar-expiry-%s", uuid.NewString()[:13])
		rr := createRemediationRequest(ROControllerNamespace, rrName)
		rar := rarSafetyRequest(rr, time.Now().Add(-1*time.Minute), true)
		// Production persists the RAR before entering AwaitingApproval; reversing
		// that order races the missing-approval safety transition.
		Expect(k8sClient.Create(ctx, rar)).To(Succeed())
		setRARSafetyParentPhase(rr.Name, remediationv1.PhaseAwaitingApproval)
		DeferCleanup(func() {
			rarSafetyRemoveFinalizer(rar.Name)
			_ = k8sClient.Delete(ctx, rr)
		})

		currentRAR := &remediationv1.RemediationApprovalRequest{}
		Eventually(func() remediationv1.ApprovalDecision {
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rar), currentRAR); err != nil {
				return remediationv1.ApprovalDecisionPending
			}
			return currentRAR.Status.Decision
		}, timeout, interval).Should(Equal(remediationv1.ApprovalDecisionExpired))

		Expect(currentRAR.Status.DecidedBy).To(Equal("system"))
		Expect(currentRAR.Status.DecidedVia).To(BeEmpty())
		Expect(currentRAR.Status.Expired).To(BeTrue())
		Expect(currentRAR.Status.DecidedAt).NotTo(BeNil())
		Expect(currentRAR.Status.DecidedAt.Time).To(BeTemporally(">=", rar.Spec.RequiredBy.Time))

		Eventually(func() *metav1.Condition {
			current := &remediationv1.RemediationApprovalRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rar), current); err != nil {
				return nil
			}
			return meta.FindStatusCondition(current.Status.Conditions, remediationapprovalrequest.ConditionAuditRecorded)
		}, timeout, interval).Should(SatisfyAll(
			Not(BeNil()),
			WithTransform(func(condition *metav1.Condition) metav1.ConditionStatus { return condition.Status }, Equal(metav1.ConditionTrue)),
		), "expiry audit bookkeeping should record enqueue acceptance")

		Eventually(func() remediationv1.RemediationPhase {
			current := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), current); err != nil {
				return ""
			}
			return current.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseFailed), "parent failure requires ExpiredConfirmed")
	})

	It("IT-RO-2564-009: RAR audit bookkeeping records enqueue acceptance, not durability", func() {
		rarName := fmt.Sprintf("rar-audit-%s", uuid.NewString()[:13])
		rar := rarSafetyRequest(nil, time.Now().Add(15*time.Minute), false)
		rar.Name = rarName
		rar.Spec.RemediationRequestRef = corev1.ObjectReference{
			APIVersion: remediationv1.GroupVersion.String(),
			Kind:       "RemediationRequest",
			Name:       "rr-audit-parent",
			Namespace:  ROControllerNamespace,
		}
		Expect(k8sClient.Create(ctx, rar)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rar) })

		decidedAt := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		Expect(approveRAR(rar.Name, func(current *remediationv1.RemediationApprovalRequest) {
			current.Status.Decision = remediationv1.ApprovalDecisionApproved
			current.Status.DecidedBy = "operator@example.com"
			current.Status.DecidedAt = &decidedAt
			current.Status.DecisionMessage = "approved for integration evidence"
		})).To(Succeed())

		current := &remediationv1.RemediationApprovalRequest{}
		Eventually(func() *metav1.Condition {
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rar), current); err != nil {
				return nil
			}
			return meta.FindStatusCondition(current.Status.Conditions, remediationapprovalrequest.ConditionAuditRecorded)
		}, timeout, interval).Should(SatisfyAll(
			Not(BeNil()),
			WithTransform(func(condition *metav1.Condition) metav1.ConditionStatus { return condition.Status }, Equal(metav1.ConditionTrue)),
		), "RAR audit controller should persist enqueue acceptance")

		Expect(meta.FindStatusCondition(current.Status.Conditions, remediationapprovalrequest.ConditionAuditRecorded).Message).
			To(ContainSubstring("accepted by audit buffer"))
		Expect(meta.FindStatusCondition(current.Status.Conditions, remediationapprovalrequest.ConditionAuditRecorded).Message).
			NotTo(ContainSubstring("recorded to DataStorage"))
	})

	DescribeTable("IT-RO-2564-017: an invalid parent identity retains the RAR finalizer",
		func(mutate func(*remediationv1.RemediationApprovalRequest)) {
			rrName := fmt.Sprintf("rr-rar-invalid-ref-%s", uuid.NewString()[:13])
			rr := createRemediationRequest(ROControllerNamespace, rrName)
			rar := rarSafetyRequest(rr, time.Now().Add(15*time.Minute), true)
			mutate(rar)
			Expect(k8sClient.Create(ctx, rar)).To(Succeed())
			setRARSafetyParentPhase(rr.Name, remediationv1.PhaseAwaitingApproval)
			DeferCleanup(func() {
				rarSafetyRemoveFinalizer(rar.Name)
				_ = k8sClient.Delete(ctx, rr)
			})

			Expect(k8sClient.Delete(ctx, rar)).To(Succeed())
			Consistently(func() bool {
				current := &remediationv1.RemediationApprovalRequest{}
				if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rar), current); err != nil {
					return false
				}
				return current.DeletionTimestamp != nil && controllerutil.ContainsFinalizer(current, creator.ApprovalRequestFinalizerName)
			}, 5*time.Second, interval).Should(BeTrue(), "invalid parent identity must fail closed and retain the finalizer")
		},
		Entry("name mismatch", func(rar *remediationv1.RemediationApprovalRequest) {
			rar.Spec.RemediationRequestRef.Name += "-different"
		}),
		Entry("reference UID mismatch", func(rar *remediationv1.RemediationApprovalRequest) {
			rar.Spec.RemediationRequestRef.UID = types.UID("different-parent-uid")
		}),
		Entry("owner UID mismatch", func(rar *remediationv1.RemediationApprovalRequest) {
			rar.OwnerReferences[0].UID = types.UID("different-owner-uid")
		}),
	)

	It("IT-RO-2564-019: real API persistence fails closed on an existing RAR", func() {
		rrName := fmt.Sprintf("rr-rar-collision-%s", uuid.NewString()[:13])
		rr := createRemediationRequest(ROControllerNamespace, rrName)
		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "ai-" + rrName, Namespace: ROControllerNamespace},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: aianalysisv1.PhaseCompleted,
				Approval: &aianalysisv1.ApprovalStatus{
					ApprovalRequired: true,
					ApprovalReason:   "low confidence",
				},
				RCAResult: &aianalysisv1.RCAResult{
					SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
						WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
							WorkflowID:             "wf-rar-collision",
							WorkflowName:           "wf-rar-collision",
							ActionType:             "RestartPod",
							Version:                "v1",
							ExecutionBundle:        "oci://example.invalid/workflows:collision",
							DeclaredParameterNames: map[string]bool{},
						},
						Confidence:      0.4,
						Rationale:       "integration collision fixture",
						Parameters:      map[string]string{},
						ParameterSchema: []remediationworkflowv1.RemediationWorkflowParameter{},
					},
				},
			},
		}
		approvalCreator := creator.NewApprovalCreator(k8sClient, k8sManager.GetScheme(), nil, 15*time.Minute)
		name, outcome, err := approvalCreator.CreateWithResult(ctx, rr, ai)
		Expect(err).NotTo(HaveOccurred())
		Expect(name).To(Equal("rar-" + rrName))
		Expect(outcome).To(Equal(creator.ApprovalCreationCreated))
		setRARSafetyParentPhase(rr.Name, remediationv1.PhaseAwaitingApproval)
		DeferCleanup(func() {
			rarSafetyRemoveFinalizer(name)
			_ = k8sClient.Delete(ctx, &remediationv1.RemediationApprovalRequest{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ROControllerNamespace}})
			_ = k8sClient.Delete(ctx, rr)
		})

		_, outcome, err = approvalCreator.CreateWithResult(ctx, rr, ai)
		Expect(err).To(HaveOccurred())
		Expect(outcome).To(Equal(creator.ApprovalCreationCollision))
	})
})

func rarSafetyRequest(rr *remediationv1.RemediationRequest, requiredBy time.Time, withFinalizer bool) *remediationv1.RemediationApprovalRequest {
	name := "rar-" + uuid.NewString()[:13]
	ownerReferences := []metav1.OwnerReference(nil)
	parentName := "rr-rar-parent"
	parentUID := types.UID("")
	if rr != nil {
		name = "rar-" + rr.Name
		parentName = rr.Name
		parentUID = rr.UID
		ownerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(rr, remediationv1.GroupVersion.WithKind("RemediationRequest"))}
	}
	finalizers := []string(nil)
	if withFinalizer {
		finalizers = []string{creator.ApprovalRequestFinalizerName}
	}
	return &remediationv1.RemediationApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       ROControllerNamespace,
			Finalizers:      finalizers,
			OwnerReferences: ownerReferences,
		},
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       parentName,
				Namespace:  ROControllerNamespace,
				UID:        parentUID,
			},
			AIAnalysisRef:   remediationv1.ObjectRef{Name: "ai-" + parentName},
			Confidence:      0.4,
			ConfidenceLevel: "low",
			Reason:          "low confidence requires approval",
			RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
				WorkflowID:      "wf-rar-safety",
				Version:         "v1",
				ExecutionBundle: "oci://example.invalid/workflows:safety",
				Rationale:       "bounded integration fixture",
			},
			InvestigationSummary: "bounded integration fixture",
			RecommendedActions: []remediationv1.ApprovalRecommendedAction{{
				Action:    "review",
				Rationale: "approval required",
			}},
			WhyApprovalRequired: "low confidence requires approval",
			RequiredBy:          metav1.NewTime(requiredBy),
		},
	}
}

func setRARSafetyParentPhase(name string, desired remediationv1.RemediationPhase) {
	Eventually(func() error {
		current := &remediationv1.RemediationRequest{}
		if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: name, Namespace: ROControllerNamespace}, current); err != nil {
			return err
		}
		current.Status.OverallPhase = desired
		current.Status.ObservedGeneration = current.Generation
		return k8sClient.Status().Update(ctx, current)
	}, timeout, interval).Should(Succeed())
}

func rarSafetyRemoveFinalizer(name string) {
	_ = retryUntilRARAvailable(name, func(current *remediationv1.RemediationApprovalRequest) error {
		if !controllerutil.ContainsFinalizer(current, creator.ApprovalRequestFinalizerName) {
			return nil
		}
		controllerutil.RemoveFinalizer(current, creator.ApprovalRequestFinalizerName)
		return k8sClient.Update(ctx, current)
	})
}

func retryUntilRARAvailable(name string, mutate func(*remediationv1.RemediationApprovalRequest) error) error {
	var lastErr error
	Eventually(func() bool {
		current := &remediationv1.RemediationApprovalRequest{}
		if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: name, Namespace: ROControllerNamespace}, current); err != nil {
			if apierrors.IsNotFound(err) {
				return true
			}
			lastErr = err
			return false
		}
		lastErr = mutate(current)
		return lastErr == nil
	}, timeout, interval).Should(BeTrue())
	return lastErr
}
