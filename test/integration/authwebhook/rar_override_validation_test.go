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

package authwebhook

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	apaudit "github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	"github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/contenthash"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/testutil"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type approvalReadBarrier struct {
	client.Client
	mu    sync.Mutex
	count int
	ready chan struct{}
}

func (b *approvalReadBarrier) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if err := b.Client.Get(ctx, key, obj, opts...); err != nil {
		return err
	}
	b.mu.Lock()
	b.count++
	if b.count == 2 {
		close(b.ready)
	}
	b.mu.Unlock()
	select {
	case <-b.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TDD Phase: RED — Issue #594, G3: Authwebhook Override Validation Integration Test
// BR-ORCH-031: Webhook rejects override referencing non-existent RW
//
// Relocated from RO integration suite (IT-RO-594-005 → IT-AW-594-005)
// because the RO suite has no admission webhook wiring.

var _ = Describe("BR-ORCH-031: RAR Override Webhook Validation (#594)", Label("integration", "override"), func() {
	var (
		ctx       context.Context
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = defaultFixture
	})

	It("IT-AF-2570-012: BR-AUTH-001 admits only the first concurrent AF decision through AuthWebhook", func() {
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "test-rar-af-race-" + randomSuffix(), Namespace: namespace},
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "test-rr", Namespace: namespace}, AIAnalysisRef: remediationv1.ObjectRef{Name: "test-analysis"},
				Confidence: 0.72, ConfidenceLevel: "medium", Reason: "review", InvestigationSummary: "review", WhyApprovalRequired: "review",
				RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{WorkflowID: "wf-test", Version: "1.0.0", ExecutionBundle: "example:v1", Rationale: "review"},
				RecommendedActions:  []remediationv1.ApprovalRecommendedAction{{Action: "review", Rationale: "review"}}, RequiredBy: metav1.NewTime(time.Now().Add(15 * time.Minute)),
				Workflow:      &sharedtypes.WorkflowSnapshot{WorkflowID: "wf-test", WorkflowName: "wf-test", ActionType: "RestartPod", Version: "1.0.0", ExecutionBundle: "example:v1"},
				Parameters:    typedStringParameters(map[string]string{"P": "2"}),
				TargetContext: &remediationv1.ApprovalTargetContext{SignalTarget: remediationv1.ResourceIdentifier{Kind: "Pod", Name: "pod"}},
			},
		}
		createAndWaitForCRD(ctx, k8sClient, rar)
		config := rest.CopyConfig(cfg)
		config.Impersonate.UserName = "system:serviceaccount:" + namespace + ":apifrontend"
		config.Impersonate.Groups = []string{"system:masters"}
		afClient, err := client.New(config, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		barrier := &approvalReadBarrier{Client: afClient, ready: make(chan struct{})}
		raceCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		errs := make(chan error, 2)
		for _, decision := range []string{"Approved", "Rejected"} {
			go func() {
				_, callErr := tools.HandleApprove(raceCtx, barrier, tools.ApproveArgs{Namespace: namespace, RARName: rar.Name, Decision: decision}, "operator-"+decision)
				errs <- callErr
			}()
		}
		first, second := <-errs, <-errs
		Expect(first == nil).NotTo(Equal(second == nil), "exactly one decision must commit")
		loser := first
		if loser == nil {
			loser = second
		}
		Expect(loser.Error()).To(ContainSubstring("conflict"))
		persisted := &remediationv1.RemediationApprovalRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rar), persisted)).To(Succeed())
		Expect(string(persisted.Status.Decision)).To(BeElementOf("Approved", "Rejected"))
		Expect(persisted.Status.DecidedBy).To(Equal("operator-" + string(persisted.Status.Decision)))
		Expect(persisted.Status.DecidedVia).To(Equal(config.Impersonate.UserName))
	})

	It("IT-AF-2570-040: BR-ORCH-025 AF review and partial approval pass real AuthWebhook admission", func() {
		correlationID := "rr-af-audit-" + randomSuffix()
		parameters := []remediationworkflowv1.RemediationWorkflowParameter{
			{Name: "P", Type: "float", Description: "threshold"},
			{Name: "Q", Type: "string", Required: true, Description: "retained value"},
		}
		rw := &remediationworkflowv1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: "test-rw-af-" + randomSuffix(), Namespace: namespace},
			Spec: remediationworkflowv1.RemediationWorkflowSpec{
				Version: "1.0.0", ActionType: "RestartPod",
				Description: remediationworkflowv1.RemediationWorkflowDescription{What: "AF review fixture", WhenToUse: "For approval testing"},
				Labels:      remediationworkflowv1.RemediationWorkflowLabels{Severity: []string{"critical"}, Environment: []string{"production"}, Component: []string{"v1/Pod"}, Priority: "P1"},
				Execution:   remediationworkflowv1.RemediationWorkflowExecution{Engine: "job", Bundle: testutil.ValidBundleRef}, Parameters: parameters,
			},
		}
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(context.Background(), rw))).To(Succeed()) })
		content, err := contenthash.MarshalCleanCRDContent(rw)
		Expect(err).NotTo(HaveOccurred())
		rw.Status.WorkflowID = contenthash.DeterministicUUID(contenthash.ComputeContentHash(string(content)))
		Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "test-rar-af-partial-" + randomSuffix(), Namespace: namespace},
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: correlationID, Namespace: namespace}, AIAnalysisRef: remediationv1.ObjectRef{Name: "test-analysis"},
				Confidence: 0.72, ConfidenceLevel: "medium", Reason: "review", InvestigationSummary: "review", WhyApprovalRequired: "review",
				RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{WorkflowID: rw.Status.WorkflowID, Version: "1.0.0", ExecutionBundle: testutil.ValidBundleRef, Rationale: "review"},
				RecommendedActions:  []remediationv1.ApprovalRecommendedAction{{Action: "review", Rationale: "review"}}, RequiredBy: metav1.NewTime(time.Now().Add(15 * time.Minute)),
				Workflow:      &sharedtypes.WorkflowSnapshot{WorkflowID: rw.Status.WorkflowID, WorkflowName: rw.Name, ActionType: "RestartPod", Version: "1.0.0", ExecutionBundle: testutil.ValidBundleRef},
				Parameters:    typedStringParameters(map[string]string{"P": "2", "Q": "keep"}),
				TargetContext: &remediationv1.ApprovalTargetContext{SignalTarget: remediationv1.ResourceIdentifier{Kind: "Pod", Name: "pod"}},
			},
		}
		createAndWaitForCRD(ctx, k8sClient, rar)
		config := rest.CopyConfig(cfg)
		config.Impersonate.UserName = "system:serviceaccount:" + namespace + ":apifrontend"
		config.Impersonate.Groups = []string{"system:masters"}
		afClient, err := client.NewWithWatch(config, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		review, err := tools.HandleGetApprovalRequest(ctx, afClient, tools.GetApprovalRequestArgs{Namespace: namespace, Name: rar.Name})
		Expect(err).NotTo(HaveOccurred())
		Expect(review.Parameters).To(Equal(map[string]interface{}{"P": "2", "Q": "keep"}))
		Expect(review.ParameterSchema).To(Equal(parameters))
		Expect(review.Workflow.WorkflowID).To(Equal(rw.Status.WorkflowID))
		roleName := "mcp-approval-" + randomSuffix()
		role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: roleName}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{"kubernaut.ai"}, Resources: []string{"console", "tools"}, Verbs: []string{"use"}}}}
		Expect(k8sClient.Create(ctx, role)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(context.Background(), role))).To(Succeed()) })
		binding := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: roleName}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: roleName}, Subjects: []rbacv1.Subject{{Kind: "User", APIGroup: rbacv1.GroupName, Name: "operator@example.com"}}}
		Expect(k8sClient.Create(ctx, binding)).To(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(context.Background(), binding))).To(Succeed()) })
		dynamicClient, err := dynamic.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		clientset, err := kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		logger := logr.Discard()
		mcp, err := handler.NewMCPHandler(handler.MCPConfig{Enabled: true, ServerName: "approval-audit-it", ServerVersion: "1.0", Bridge: &handler.MCPBridgeConfig{
			K8sClient: dynamicClient, TypedClient: afClient, Namespace: namespace, Authorizer: auth.NewSARChecker(clientset, time.Second, logger), Auditor: apaudit.NewStoreAdapter(auditStore, logger), Logger: logger, ToolTimeout: 5 * time.Second, MaxConcurrentTools: 2,
		}})
		Expect(err).NotTo(HaveOccurred())
		user := &auth.UserIdentity{Username: "operator@example.com", Issuer: "integration-identity"}
		post := func(session, body string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			if session != "" {
				req.Header.Set("Mcp-Session-Id", session)
			}
			req = req.WithContext(auth.WithUserIdentity(req.Context(), user))
			response := httptest.NewRecorder()
			mcp.ServeHTTP(response, req)
			return response
		}
		user = &auth.UserIdentity{Username: "denied-" + randomSuffix(), Issuer: "integration-identity"}
		deniedInit := post("", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"denied-it","version":"1"}}}`)
		Expect(deniedInit.Code).To(Equal(http.StatusOK))
		deniedSession := deniedInit.Header().Get("Mcp-Session-Id")
		Expect(deniedSession).NotTo(BeEmpty())
		post(deniedSession, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
		deniedRead := post(deniedSession, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"kubernaut_get_approval_request","arguments":{"name":%q}}}`, rar.Name))
		Expect(deniedRead.Body.String()).To(ContainSubstring("permission denied"))
		denied := post(deniedSession, fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"kubernaut_approve","arguments":{"rar_name":%q,"decision":"Approved"}}}`, rar.Name))
		Expect(denied.Body.String()).To(ContainSubstring("permission denied"))
		before := &remediationv1.RemediationApprovalRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rar), before)).To(Succeed())
		Expect(before.Status.Decision).To(Equal(remediationv1.ApprovalDecisionPending))
		user = &auth.UserIdentity{Username: "operator@example.com", Issuer: "integration-identity"}
		initialized := post("", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"audit-it","version":"1"}}}`)
		Expect(initialized.Code).To(Equal(http.StatusOK))
		session := initialized.Header().Get("Mcp-Session-Id")
		Expect(session).NotTo(BeEmpty())
		post(session, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
		readResponse := post(session, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"kubernaut_get_approval_request","arguments":{"name":%q}}}`, rar.Name))
		Expect(readResponse.Code).To(Equal(http.StatusOK))
		Expect(readResponse.Body.String()).NotTo(ContainSubstring(`"isError":true`))
		Expect(readResponse.Body.String()).To(ContainSubstring("threshold"))
		Expect(readResponse.Body.String()).To(ContainSubstring("keep"))
		response := post(session, fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"kubernaut_approve","arguments":{"rar_name":%q,"decision":"Approved","workflow_override":{"parameters":{"P":"1.0"},"rationale":"reviewed"}}}}`, rar.Name))
		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(response.Body.String()).NotTo(ContainSubstring(`"isError":true`))
		persisted := &remediationv1.RemediationApprovalRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rar), persisted)).To(Succeed())
		Expect(persisted.Status.Decision).To(Equal(remediationv1.ApprovalDecisionApproved))
		Expect(persisted.Status.DecidedBy).To(Equal("operator@example.com"))
		Expect(persisted.Status.WorkflowOverride.Parameters).To(Equal(typedStringParameters(map[string]string{"P": "1.0"})))
		Expect(persisted.Spec.Parameters).To(Equal(typedStringParameters(map[string]string{"P": "2", "Q": "keep"})))
		Eventually(func(g Gomega) {
			events, queryErr := queryAuditEvents(dsClient, correlationID, nil)
			g.Expect(queryErr).NotTo(HaveOccurred())
			found := false
			for _, event := range events {
				payload, ok := event.EventData.GetRemediationApprovalAuditPayload()
				if !ok || payload.RequestName != rar.Name {
					continue
				}
				original, originalPresent := payload.OriginalParameters.Get()
				changes, changesPresent := payload.OverrideParameters.Get()
				effective, effectivePresent := payload.EffectiveParameters.Get()
				g.Expect(originalPresent && changesPresent && effectivePresent).To(BeTrue())
				g.Expect(original).To(HaveKeyWithValue("P", "2"))
				g.Expect(changes).To(HaveKeyWithValue("P", "1.0"))
				g.Expect(effective).To(HaveKeyWithValue("P", "1.0"))
				g.Expect(effective).To(HaveKeyWithValue("Q", "keep"))
				g.Expect(payload.WorkflowID.Value).To(Equal(rw.Status.WorkflowID))
				g.Expect(payload.OverrideRationale.Value).To(Equal("reviewed"))
				g.Expect(payload.DelegatedUser.Value).To(Equal("operator@example.com"))
				g.Expect(payload.DelegatedVia.Value).To(Equal(config.Impersonate.UserName))
				g.Expect(event.EventID.IsSet()).To(BeTrue())
				g.Expect(event.CorrelationID).To(Equal(correlationID))
				g.Expect(event.EventAction).To(Equal("approval_decided"))
				g.Expect(string(event.EventOutcome)).To(Equal("success"))
				g.Expect(event.ActorID.Value).To(Equal(config.Impersonate.UserName))
				found = true
				break
			}
			g.Expect(found).To(BeTrue(), "the admitted decision must be persisted in DataStorage")
		}, 30*time.Second, 250*time.Millisecond).Should(Succeed())

		roConfig := rest.CopyConfig(cfg)
		roConfig.Impersonate.UserName = "system:serviceaccount:" + namespace + ":remediationorchestrator-controller"
		roConfig.Impersonate.Groups = []string{"system:masters"}
		roClient, err := client.New(roConfig, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		ro := controller.NewRARReconciler(roClient, roClient, k8sClient.Scheme(), auditStore, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
		_, err = ro.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(rar)})
		Expect(err).NotTo(HaveOccurred())
		Expect(roClient.Get(ctx, client.ObjectKeyFromObject(rar), persisted)).To(Succeed())
		auditCondition := meta.FindStatusCondition(persisted.Status.Conditions, remediationapprovalrequest.ConditionAuditRecorded)
		Expect(auditCondition).NotTo(BeNil())
		Expect(auditCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(auditCondition.Message).To(ContainSubstring("accepted by audit buffer"))
		Eventually(func(g Gomega) {
			g.Expect(auditStore.Flush(ctx)).To(Succeed())
			events, queryErr := queryAuditEvents(dsClient, correlationID, nil)
			g.Expect(queryErr).NotTo(HaveOccurred())
			found, foundIntent := false, false
			for _, event := range events {
				if event.EventType == "apifrontend.tool.executed" {
					intent, ok := event.EventData.GetApifrontendToolExecutedPayload()
					g.Expect(ok).To(BeTrue())
					if intent.ToolName != "kubernaut_approve" {
						continue
					}
					original, originalPresent := intent.OriginalParameters.Get()
					changes, changesPresent := intent.OverrideParameters.Get()
					effective, effectivePresent := intent.EffectiveParameters.Get()
					g.Expect(originalPresent && changesPresent && effectivePresent).To(BeTrue())
					g.Expect(original).To(HaveKeyWithValue("P", "2"))
					g.Expect(changes).To(HaveKeyWithValue("P", "1.0"))
					g.Expect(effective).To(HaveKeyWithValue("P", "1.0"))
					g.Expect(effective).To(HaveKeyWithValue("Q", "keep"))
					g.Expect(intent.WorkflowID.Value).To(Equal(rw.Status.WorkflowID))
					g.Expect(event.ActorID.Value).To(Equal("operator@example.com"))
					foundIntent = true
					continue
				}
				if event.EventType != "orchestrator.approval.approved" {
					continue
				}
				payload, ok := event.EventData.GetRemediationOrchestratorAuditPayload()
				g.Expect(ok).To(BeTrue())
				original, originalPresent := payload.OriginalParameters.Get()
				changes, changesPresent := payload.OverrideParameters.Get()
				effective, effectivePresent := payload.EffectiveParameters.Get()
				g.Expect(originalPresent && changesPresent && effectivePresent).To(BeTrue())
				g.Expect(original).To(HaveKeyWithValue("P", "2"))
				g.Expect(changes).To(HaveKeyWithValue("P", "1.0"))
				g.Expect(effective).To(HaveKeyWithValue("P", "1.0"))
				g.Expect(effective).To(HaveKeyWithValue("Q", "keep"))
				g.Expect(payload.WorkflowID.Value).To(Equal(rw.Status.WorkflowID))
				g.Expect(event.ActorID.Value).To(Equal("operator@example.com"))
				found = true
			}
			g.Expect(found).To(BeTrue(), "the committed RO decision must be persisted under the same correlation ID")
			g.Expect(foundIntent).To(BeTrue(), "the MCP approval intent must be persisted under the same correlation ID")
		}, 30*time.Second, 250*time.Millisecond).Should(Succeed())
	})

	It("IT-2570-021: BR-ORCH-025 rejects approval of a legacy RAR without snapshot metadata", func() {
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "test-rar-legacy-" + randomSuffix(), Namespace: namespace},
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "test-rr", Namespace: namespace}, AIAnalysisRef: remediationv1.ObjectRef{Name: "test-analysis"},
				Confidence: 0.72, ConfidenceLevel: "medium", Reason: "review", InvestigationSummary: "review", WhyApprovalRequired: "review",
				RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{WorkflowID: "wf-original", Version: "v1", ExecutionBundle: "example:v1", Rationale: "review"},
				RecommendedActions:  []remediationv1.ApprovalRecommendedAction{{Action: "review", Rationale: "review"}}, RequiredBy: metav1.NewTime(metav1.Now().Add(15 * 60 * 1000000000)),
			},
		}
		createAndWaitForCRD(ctx, k8sClient, rar)
		rar.Status.Decision = remediationv1.ApprovalDecisionApproved
		err := k8sClient.Status().Update(ctx, rar)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("snapshot"))
		persisted := &remediationv1.RemediationApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rar), persisted)).To(Succeed())
		Expect(persisted.Status.Decision).To(Equal(remediationv1.ApprovalDecisionPending))
	})

	It("IT-AW-2570-004: BR-KA-191 rejects a decimal integer before a decision is persisted", func() {
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "test-rar-schema-" + randomSuffix(), Namespace: namespace},
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "test-rr", Namespace: namespace},
				AIAnalysisRef:         remediationv1.ObjectRef{Name: "test-analysis"},
				Confidence:            0.72, ConfidenceLevel: "medium", Reason: "Below threshold",
				InvestigationSummary: "Test investigation", WhyApprovalRequired: "Medium confidence",
				RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{WorkflowID: "wf-original", Version: "1.0.0", ExecutionBundle: "original-bundle:v1", Rationale: "AI selected"},
				RecommendedActions:  []remediationv1.ApprovalRecommendedAction{{Action: "Restart", Rationale: "OOM recovery"}},
				RequiredBy:          metav1.NewTime(metav1.Now().Add(15 * 60 * 1000000000)),
				Workflow:            &sharedtypes.WorkflowSnapshot{WorkflowID: "wf-original", WorkflowName: "wf-original", ActionType: "RestartPod", Version: "1.0.0", ExecutionBundle: "original-bundle:v1"},
				Parameters:          typedStringParameters(map[string]string{"P": "3"}),
				TargetContext:       &remediationv1.ApprovalTargetContext{SignalTarget: remediationv1.ResourceIdentifier{Kind: "Pod", Name: "test-pod"}},
			},
		}
		createAndWaitForCRD(ctx, k8sClient, rar)
		rar.Status.Decision = remediationv1.ApprovalDecisionApproved
		rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{Parameters: typedStringParameters(map[string]string{"P": "1.0"})}
		err := k8sClient.Status().Update(ctx, rar)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("P"))
		persisted := &remediationv1.RemediationApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rar), persisted)).To(Succeed())
		Expect(persisted.Status.Decision).To(Equal(remediationv1.ApprovalDecisionPending))
		Expect(persisted.Status.WorkflowOverride).To(BeNil())
	})

	It("IT-AW-2570-028: BR-AUTH-001 rejects a valid override when the trusted workflow ID is absent", func() {
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "test-rar-id-" + randomSuffix(), Namespace: namespace},
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "test-rr", Namespace: namespace}, AIAnalysisRef: remediationv1.ObjectRef{Name: "test-analysis"},
				Confidence: 0.72, ConfidenceLevel: "medium", Reason: "review", InvestigationSummary: "review", WhyApprovalRequired: "review",
				RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{WorkflowID: "wf-missing", Version: "v1", ExecutionBundle: "example:v1", Rationale: "review"},
				RecommendedActions:  []remediationv1.ApprovalRecommendedAction{{Action: "review", Rationale: "review"}}, RequiredBy: metav1.NewTime(metav1.Now().Add(15 * 60 * 1000000000)),
				Workflow:      &sharedtypes.WorkflowSnapshot{WorkflowID: "wf-missing", WorkflowName: "wf-missing", ActionType: "RestartPod", Version: "v1", ExecutionBundle: "example:v1"},
				Parameters:    typedStringParameters(map[string]string{"P": "3"}),
				TargetContext: &remediationv1.ApprovalTargetContext{SignalTarget: remediationv1.ResourceIdentifier{Kind: "Pod", Name: "pod"}},
			},
		}
		createAndWaitForCRD(ctx, k8sClient, rar)
		rar.Status.Decision = remediationv1.ApprovalDecisionApproved
		rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{Parameters: typedStringParameters(map[string]string{"P": "4"})}
		err := k8sClient.Status().Update(ctx, rar)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("workflow"))
		persisted := &remediationv1.RemediationApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rar), persisted)).To(Succeed())
		Expect(persisted.Status.Decision).To(Equal(remediationv1.ApprovalDecisionPending))
	})

	It("IT-AW-2570-011: BR-ORCH-025 preserves an explicitly empty override on the admission path", func() {
		rw := &remediationworkflowv1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: "test-rw-override-" + randomSuffix(), Namespace: namespace},
			Spec: remediationworkflowv1.RemediationWorkflowSpec{
				Version: "1.0.0", ActionType: "RestartPod",
				Description: remediationworkflowv1.RemediationWorkflowDescription{What: "Test workflow", WhenToUse: "For approval testing"},
				Labels:      remediationworkflowv1.RemediationWorkflowLabels{Severity: []string{"critical"}, Environment: []string{"production"}, Component: []string{"v1/Pod"}, Priority: "P1"},
				Execution:   remediationworkflowv1.RemediationWorkflowExecution{Engine: "job", Bundle: testutil.ValidBundleRef},
				Parameters:  []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "float", Description: "threshold"}},
			},
		}
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rw) })
		content, err := contenthash.MarshalCleanCRDContent(rw)
		Expect(err).NotTo(HaveOccurred())
		workflowID := contenthash.DeterministicUUID(contenthash.ComputeContentHash(string(content)))
		rw.Status.WorkflowID = workflowID
		Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "test-rar-empty-override-" + randomSuffix(), Namespace: namespace},
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "test-rr", Namespace: namespace},
				AIAnalysisRef:         remediationv1.ObjectRef{Name: "test-analysis"},
				Confidence:            0.72, ConfidenceLevel: "medium", Reason: "Below threshold",
				InvestigationSummary: "Test investigation", WhyApprovalRequired: "Medium confidence",
				RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{WorkflowID: "wf-original", Version: "1.0.0", ExecutionBundle: "original-bundle:v1", Rationale: "AI selected"},
				RecommendedActions:  []remediationv1.ApprovalRecommendedAction{{Action: "Restart", Rationale: "OOM recovery"}},
				RequiredBy:          metav1.NewTime(metav1.Now().Add(15 * 60 * 1000000000)),
				Workflow:            &sharedtypes.WorkflowSnapshot{WorkflowID: workflowID, WorkflowName: rw.Name, ActionType: "RestartPod", Version: "1.0.0", ExecutionBundle: testutil.ValidBundleRef},
				Parameters:          typedStringParameters(map[string]string{"P": "2"}),
				TargetContext:       &remediationv1.ApprovalTargetContext{SignalTarget: remediationv1.ResourceIdentifier{Kind: "Pod", Name: "pod"}},
			},
		}
		createAndWaitForCRD(ctx, k8sClient, rar)
		rar.Status.Decision = remediationv1.ApprovalDecisionApproved
		rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{Parameters: typedStringParameters(map[string]string{})}
		Expect(k8sClient.Status().Update(ctx, rar)).To(Succeed())
		persisted := &remediationv1.RemediationApprovalRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rar), persisted)).To(Succeed())
		Expect(persisted.Status.WorkflowOverride).NotTo(BeNil())
		Expect(persisted.Status.WorkflowOverride.Parameters).NotTo(BeNil())
		Expect(persisted.Status.WorkflowOverride.Parameters).To(BeEmpty())

		changed := &remediationv1.RemediationApprovalRequest{ObjectMeta: metav1.ObjectMeta{Name: "test-rar-valid-override-" + randomSuffix(), Namespace: namespace}, Spec: *rar.Spec.DeepCopy()}
		createAndWaitForCRD(ctx, k8sClient, changed)
		changed.Status.Decision = remediationv1.ApprovalDecisionApproved
		changed.Status.WorkflowOverride = &remediationv1.WorkflowOverride{Parameters: typedStringParameters(map[string]string{"P": "1.0"})}
		Expect(k8sClient.Status().Update(ctx, changed)).To(Succeed())
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(changed), persisted)).To(Succeed())
		Expect(persisted.Status.WorkflowOverride.Parameters).To(Equal(typedStringParameters(map[string]string{"P": "1.0"})))
	})

	Context("IT-AW-594-005: Override referencing non-existent RW → webhook denies", func() {
		It("should reject the status update when override references a RW that does not exist", func() {
			By("Creating a RAR without override (initial state)")
			rar := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rar-override-deny-" + randomSuffix(),
					Namespace: namespace,
				},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-rr",
						Namespace: namespace,
					},
					AIAnalysisRef: remediationv1.ObjectRef{
						Name: "test-analysis",
					},
					Confidence:           0.72,
					ConfidenceLevel:      "medium",
					Reason:               "Below threshold",
					InvestigationSummary: "Test investigation",
					WhyApprovalRequired:  "Medium confidence",
					RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
						WorkflowID:      "wf-original",
						Version:         "1.0.0",
						ExecutionBundle: "original-bundle:v1",
						Rationale:       "AI selected",
					},
					RecommendedActions: []remediationv1.ApprovalRecommendedAction{
						{Action: "Restart", Rationale: "OOM recovery"},
					},
					RequiredBy: metav1.NewTime(metav1.Now().Add(15 * 60 * 1000000000)),
				},
			}
			createAndWaitForCRD(ctx, k8sClient, rar)

			By("Updating status with override referencing non-existent RW")
			rar.Status.Decision = remediationv1.ApprovalDecisionApproved
			rar.Status.DecisionMessage = "Override to non-existent workflow"
			rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{
				WorkflowName: "this-workflow-does-not-exist",
				Rationale:    "testing webhook rejection",
			}

			err := k8sClient.Status().Update(ctx, rar)
			Expect(err).To(HaveOccurred(), "webhook should deny override with non-existent RW")
			Expect(err.Error()).To(ContainSubstring("workflow switching"),
				"workflow switching is rejected before catalog lookup")
		})
	})

	It("IT-AW-2570-011: BR-ORCH-025 rejects workflow switching before any catalog lookup", func() {
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "test-rar-switch-" + randomSuffix(), Namespace: namespace},
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "test-rr", Namespace: namespace},
				AIAnalysisRef:         remediationv1.ObjectRef{Name: "test-analysis"},
				Confidence:            0.72, ConfidenceLevel: "medium", Reason: "Below threshold",
				InvestigationSummary: "Test investigation", WhyApprovalRequired: "Medium confidence",
				RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{WorkflowID: "wf-original", Version: "1.0.0", ExecutionBundle: "original-bundle:v1", Rationale: "AI selected"},
				RecommendedActions:  []remediationv1.ApprovalRecommendedAction{{Action: "Restart", Rationale: "OOM recovery"}},
				RequiredBy:          metav1.NewTime(metav1.Now().Add(15 * 60 * 1000000000)),
			},
		}
		createAndWaitForCRD(ctx, k8sClient, rar)
		rar.Status.Decision = remediationv1.ApprovalDecisionApproved
		rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{WorkflowName: "nonexistent-workflow"}
		err := k8sClient.Status().Update(ctx, rar)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("workflow switching"))
		persisted := &remediationv1.RemediationApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rar), persisted)).To(Succeed())
		Expect(persisted.Status.Decision).To(Equal(remediationv1.ApprovalDecisionPending))
	})

	It("IT-AW-2570-011: BR-AUTH-001 denies a trusted system cancellation with an override", func() {
		config := rest.CopyConfig(cfg)
		config.Impersonate.UserName = "system:serviceaccount:default:remediationorchestrator-controller"
		config.Impersonate.Groups = []string{"system:masters"}
		systemClient, err := client.New(config, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		newRAR := func() *remediationv1.RemediationApprovalRequest {
			rar := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "test-rar-cancel-" + randomSuffix(), Namespace: namespace},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					RemediationRequestRef: corev1.ObjectReference{Name: "test-rr", Namespace: namespace},
					AIAnalysisRef:         remediationv1.ObjectRef{Name: "test-analysis"},
					Confidence:            0.72, ConfidenceLevel: "medium", Reason: "Below threshold",
					InvestigationSummary: "Test investigation", WhyApprovalRequired: "Medium confidence",
					RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{WorkflowID: "wf-original", Version: "1.0.0", ExecutionBundle: "original-bundle:v1", Rationale: "AI selected"},
					RecommendedActions:  []remediationv1.ApprovalRecommendedAction{{Action: "Restart", Rationale: "OOM recovery"}},
					RequiredBy:          metav1.NewTime(metav1.Now().Add(15 * 60 * 1000000000)),
				},
			}
			createAndWaitForCRD(ctx, k8sClient, rar)
			return rar
		}
		control := newRAR()
		control.Status.Decision = remediationv1.ApprovalDecisionCancelled
		control.Status.DecidedBy = "system"
		Expect(systemClient.Status().Update(ctx, control)).To(Succeed(), "the trusted cancellation without an override is valid")

		rar := newRAR()
		rar.Status.Decision = remediationv1.ApprovalDecisionCancelled
		rar.Status.DecidedBy = "system"
		rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{Rationale: "not permitted"}
		err = systemClient.Status().Update(ctx, rar)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("override"))
		persisted := &remediationv1.RemediationApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rar), persisted)).To(Succeed())
		Expect(persisted.Status.Decision).To(Equal(remediationv1.ApprovalDecisionPending))
		Expect(persisted.Status.WorkflowOverride).To(BeNil())
	})

	It("IT-AW-2570-011: BR-AUTH-001 rejects an override on pending status", func() {
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "test-rar-pending-" + randomSuffix(), Namespace: namespace},
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "test-rr", Namespace: namespace},
				AIAnalysisRef:         remediationv1.ObjectRef{Name: "test-analysis"},
				Confidence:            0.72, ConfidenceLevel: "medium", Reason: "Below threshold",
				InvestigationSummary: "Test investigation", WhyApprovalRequired: "Medium confidence",
				RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{WorkflowID: "wf-original", Version: "1.0.0", ExecutionBundle: "original-bundle:v1", Rationale: "AI selected"},
				RecommendedActions:  []remediationv1.ApprovalRecommendedAction{{Action: "Restart", Rationale: "OOM recovery"}},
				RequiredBy:          metav1.NewTime(metav1.Now().Add(15 * 60 * 1000000000)),
			},
		}
		createAndWaitForCRD(ctx, k8sClient, rar)
		rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{Rationale: "not permitted"}
		err := k8sClient.Status().Update(ctx, rar)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("override"))
		persisted := &remediationv1.RemediationApprovalRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rar), persisted)).To(Succeed())
		Expect(persisted.Status.WorkflowOverride).To(BeNil())
	})
})
