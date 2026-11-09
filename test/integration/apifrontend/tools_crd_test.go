package apifrontend_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// decisionReadBarrier forces both callers to read the same pending resourceVersion.
type decisionReadBarrier struct {
	client.Client
	reads atomic.Int32
	ready chan struct{}
}

func (c *decisionReadBarrier) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if err := c.Client.Get(ctx, key, obj, opts...); err != nil {
		return err
	}
	if c.reads.Add(1) == 2 {
		close(c.ready)
	}
	select {
	case <-c.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

var _ = Describe("CRD Tools Integration (tools/ via envtest)", func() {

	rrGVR := schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}

	Describe("AC-24: CRD tools against real K8s API", func() {
		It("IT-AF-2570-012: BR-AUTH-001 concurrent decisions cannot overwrite the first committed decision", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			rar := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rar-2570-concurrent", Namespace: defaultFixture},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					RemediationRequestRef: corev1.ObjectReference{Name: "rr-test", Namespace: defaultFixture}, AIAnalysisRef: remediationv1.ObjectRef{Name: "ai-test"},
					Confidence: 0.7, ConfidenceLevel: "medium", Reason: "review", InvestigationSummary: "review", WhyApprovalRequired: "review",
					RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{WorkflowID: "wf-test", Version: "v1", ExecutionBundle: "example:v1", Rationale: "review"},
					RecommendedActions:  []remediationv1.ApprovalRecommendedAction{{Action: "review", Rationale: "review"}}, RequiredBy: metav1.NewTime(time.Now().Add(15 * time.Minute)),
					Workflow:   &sharedtypes.WorkflowSnapshot{WorkflowID: "wf-test", WorkflowName: "wf-test", ActionType: "RestartPod", Version: "v1", ExecutionBundle: "example:v1", DeclaredParameterNames: map[string]bool{"P": true}},
					Parameters: map[string]string{"P": "2"}, ParameterSchema: []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "float", Description: "threshold"}},
					TargetContext: &remediationv1.ApprovalTargetContext{SignalTarget: remediationv1.ResourceIdentifier{Kind: "Pod", Name: "pod"}},
				},
			}
			Expect(k8sClient.Create(ctx, rar)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(context.Background(), rar) })
			barrier := &decisionReadBarrier{Client: k8sClient, ready: make(chan struct{})}
			errs := make(chan error, 2)
			for _, decision := range []string{"Approved", "Rejected"} {
				go func() {
					_, err := tools.HandleApprove(ctx, barrier, tools.ApproveArgs{Namespace: defaultFixture, RARName: rar.Name, Decision: decision}, "operator-"+decision)
					errs <- err
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
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rar), persisted)).To(Succeed())
			Expect(string(persisted.Status.Decision)).To(BeElementOf("Approved", "Rejected"))
			Expect(persisted.Status.DecidedBy).To(Equal("operator-" + string(persisted.Status.Decision)))
		})

		It("IT-AF-2570-003: BR-ORCH-025 persists a structured parameter-only approval override", func() {
			ctx := context.Background()
			rar := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rar-2570-structured", Namespace: defaultFixture},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					RemediationRequestRef: corev1.ObjectReference{Name: "rr-test", Namespace: defaultFixture}, AIAnalysisRef: remediationv1.ObjectRef{Name: "ai-test"},
					Confidence: 0.7, ConfidenceLevel: "medium", Reason: "review", InvestigationSummary: "review", WhyApprovalRequired: "review",
					RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{WorkflowID: "wf-test", Version: "v1", ExecutionBundle: "example:v1", Rationale: "review"},
					RecommendedActions:  []remediationv1.ApprovalRecommendedAction{{Action: "review", Rationale: "review"}}, RequiredBy: metav1.NewTime(time.Now().Add(15 * time.Minute)),
					Workflow:   &sharedtypes.WorkflowSnapshot{WorkflowID: "wf-test", WorkflowName: "wf-test", ActionType: "RestartPod", Version: "v1", ExecutionBundle: "example:v1", DeclaredParameterNames: map[string]bool{"P": true}},
					Parameters: map[string]string{"P": "2"}, ParameterSchema: []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "float", Description: "threshold"}},
					TargetContext: &remediationv1.ApprovalTargetContext{SignalTarget: remediationv1.ResourceIdentifier{Kind: "Pod", Name: "pod"}},
				},
			}
			Expect(k8sClient.Create(ctx, rar)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rar) })
			readback, err := tools.HandleGetApprovalRequest(ctx, k8sClient, tools.GetApprovalRequestArgs{Namespace: defaultFixture, Name: rar.Name})
			Expect(err).NotTo(HaveOccurred())
			encoded, err := json.Marshal(readback)
			Expect(err).NotTo(HaveOccurred())
			var detail map[string]json.RawMessage
			Expect(json.Unmarshal(encoded, &detail)).To(Succeed())
			Expect(string(detail["parameters"])).To(MatchJSON(`{"P":"2"}`))
			Expect(string(detail["parameter_schema"])).To(ContainSubstring(`"name":"P"`))
			var args tools.ApproveArgs
			Expect(json.Unmarshal([]byte(`{"rar_name":"rar-2570-structured","decision":"Approved","workflow_override":{"parameters":{"P":"1.0"},"rationale":"reviewed"}}`), &args)).To(Succeed())
			args.Namespace = defaultFixture
			_, err = tools.HandleApprove(ctx, k8sClient, args, "operator@example.com")
			Expect(err).NotTo(HaveOccurred())
			persisted := &remediationv1.RemediationApprovalRequest{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rar), persisted)).To(Succeed())
			Expect(persisted.Status.WorkflowOverride.Parameters).To(HaveKeyWithValue("P", "1.0"))
			Expect(persisted.Status.WorkflowOverride.Rationale).To(Equal("reviewed"))
		})
		It("IT-AF-1195-035: list_remediations returns list from envtest", func() {
			ctx := context.Background()

			// Create a test RR CRD in envtest
			rr := &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "kubernaut.ai/v1alpha1",
					"kind":       "RemediationRequest",
					"metadata": map[string]any{
						"name":      "test-rr-035",
						"namespace": defaultFixture,
					},
					"spec": map[string]any{
						"signalName":        "TestAlert",
						"signalFingerprint": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
						"signalType":        "prometheus",
						"severity":          "warning",
						"firingTime":        "2025-01-01T00:00:00Z",
						"receivedTime":      "2025-01-01T00:00:01Z",
						"targetType":        "kubernetes",
						"targetResource": map[string]any{
							"kind": "Deployment",
							"name": "test-app",
						},
					},
				},
			}
			_, err := dynamicClient.Resource(rrGVR).Namespace(defaultFixture).Create(ctx, rr, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_ = dynamicClient.Resource(rrGVR).Namespace(defaultFixture).Delete(ctx, "test-rr-035", metav1.DeleteOptions{})
			})

			result, err := tools.HandleListRemediations(ctx, k8sClient, tools.ListRemediationsArgs{
				Namespace: defaultFixture,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(BeNumerically(">=", 1))
		})

		It("IT-AF-1195-036: kubectl_list returns pod list from envtest", func() {
			ctx := context.Background()

			result, err := tools.HandleKubectlList(ctx, &tools.DynamicResourceReader{Client: dynamicClient}, nil, tools.KubectlListArgs{
				Kind:      "Pod",
				Namespace: defaultFixture,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Items).NotTo(BeNil())
		})

		It("IT-AF-1195-037: kubectl_list returns workload list from envtest", func() {
			ctx := context.Background()

			result, err := tools.HandleKubectlList(ctx, &tools.DynamicResourceReader{Client: dynamicClient}, nil, tools.KubectlListArgs{
				Kind:      "Deployment",
				Namespace: defaultFixture,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Items).NotTo(BeNil())
		})
	})

	Describe("AC-27: RBAC enforcement via AF SA (ADR-022)", func() {
		It("IT-AF-1195-041: AF SA client can list remediations (K8s RBAC is per-SA, not per-user)", func() {
			factory := auth.StaticDynamicFactory(dynamicClient)
			Expect(factory).NotTo(BeNil())

			ctx := context.Background()
			client, err := factory(ctx)
			Expect(err).NotTo(HaveOccurred())

			// AF SA has read permissions on remediationrequests (ADR-022)
			_, err = client.Resource(rrGVR).Namespace(defaultFixture).List(ctx, metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred(), "AF SA should have permission to list remediations")
		})
	})
})
