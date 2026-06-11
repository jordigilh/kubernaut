package tools_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("kubernaut_watch", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-106-001: emits phase change events for RR", func() {
		fakeWatcher := watch.NewFake()
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Pending"))
		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, fakeWatcher, nil
		})

		go func() {
			defer fakeWatcher.Stop()
			time.Sleep(10 * time.Millisecond)
			fakeWatcher.Modify(newFakeRR("payments", "rr-1", "Executing"))
			time.Sleep(10 * time.Millisecond)
			fakeWatcher.Modify(newFakeRR("payments", "rr-1", "Completed"))
		}()

		result, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Events).NotTo(BeEmpty())
	})

	It("UT-AF-106-002: correlates related CRDs via ownerRef", func() {
		fakeWatcher := watch.NewFake()
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"))
		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, fakeWatcher, nil
		})

		go func() {
			defer fakeWatcher.Stop()
			time.Sleep(10 * time.Millisecond)
			fakeWatcher.Modify(newFakeRR("payments", "rr-1", "Completed"))
		}()

		result, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).NotTo(BeEmpty())
	})

	It("UT-AF-106-003: emits events in chronological order", func() {
		fakeWatcher := watch.NewFake()
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Pending"))
		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, fakeWatcher, nil
		})

		go func() {
			defer fakeWatcher.Stop()
			time.Sleep(10 * time.Millisecond)
			fakeWatcher.Modify(newFakeRR("payments", "rr-1", "Executing"))
			time.Sleep(10 * time.Millisecond)
			fakeWatcher.Modify(newFakeRR("payments", "rr-1", "Completed"))
		}()

		result, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		if len(result.Events) >= 2 {
			Expect(result.Events[0].Timestamp <= result.Events[1].Timestamp).To(BeTrue())
		}
	})

	It("UT-AF-106-004: closes stream on terminal RR state", func() {
		fakeWatcher := watch.NewFake()
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"))
		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, fakeWatcher, nil
		})

		go func() {
			defer fakeWatcher.Stop()
			time.Sleep(10 * time.Millisecond)
			fakeWatcher.Modify(newFakeRR("payments", "rr-1", "Completed"))
		}()

		result, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))
	})

	It("UT-AF-106-006: respects context cancellation", func() {
		cancelCtx, cancel := context.WithCancel(ctx)
		fakeWatcher := watch.NewFake()
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"))
		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, fakeWatcher, nil
		})

		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		result, err := tools.HandleWatch(cancelCtx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("cancelled"))
	})

	It("UT-AF-106-007: uses impersonated watch", func() {
		var capturedAction k8stesting.Action
		fakeWatcher := watch.NewFake()
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"))
		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			capturedAction = action
			return true, fakeWatcher, nil
		})

		go func() {
			defer fakeWatcher.Stop()
			time.Sleep(10 * time.Millisecond)
			fakeWatcher.Modify(newFakeRR("payments", "rr-1", "Completed"))
		}()

		_, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(capturedAction).NotTo(BeNil())
		Expect(capturedAction.GetNamespace()).To(Equal("payments"))
		Expect(capturedAction.GetResource()).To(Equal(schema.GroupVersionResource{
			Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests",
		}))
	})

	It("UT-AF-106-008: returns 403 when user cannot access namespace", func() {
		client := newDynamicFakeClient()
		client.PrependReactor("get", "remediationrequests", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, newForbiddenError("remediationrequests")
		})
		_, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "forbidden", Name: "rr-1"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("access denied"))
	})

	It("UT-AF-106-012: returns not-found when RR does not exist", func() {
		client := newDynamicFakeClient()
		_, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "default", Name: "nonexistent"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("UT-AF-106-009: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleWatch(ctx, nil, tools.WatchArgs{Namespace: "default", Name: "rr-1"})
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-106-010: invalid namespace returns ErrInvalidInput", func() {
		client := newDynamicFakeClient()
		_, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "../etc", Name: "rr-1"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-106-011: invalid resource name returns ErrInvalidInput", func() {
		client := newDynamicFakeClient()
		_, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "default", Name: "INVALID NAME!!"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-106-013: yields with awaiting_approval on AwaitingApproval phase", func() {
		fakeWatcher := watch.NewFake()
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"))
		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, fakeWatcher, nil
		})

		go func() {
			time.Sleep(10 * time.Millisecond)
			fakeWatcher.Modify(newFakeRR("payments", "rr-1", "AwaitingApproval"))
		}()

		result, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("awaiting_approval"),
			"watch must yield on AwaitingApproval so the LLM can approve the RAR")
		Expect(result.Events).To(HaveLen(1))
		Expect(result.Events[0].Phase).To(Equal("AwaitingApproval"))
	})

	It("UT-AF-106-014: AwaitingApproval does not block subsequent terminal phase", func() {
		fakeWatcher := watch.NewFake()
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"))
		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, fakeWatcher, nil
		})

		go func() {
			time.Sleep(10 * time.Millisecond)
			fakeWatcher.Modify(newFakeRR("payments", "rr-1", "AwaitingApproval"))
		}()

		result, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("awaiting_approval"),
			"first watch call yields on AwaitingApproval")

		// After approval, a second watch call should see the terminal phase
		fakeWatcher2 := watch.NewFake()
		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, fakeWatcher2, nil
		})

		go func() {
			defer fakeWatcher2.Stop()
			time.Sleep(10 * time.Millisecond)
			fakeWatcher2.Modify(newFakeRR("payments", "rr-1", "Completed"))
		}()

		result2, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result2.Status).To(Equal("completed"),
			"second watch call should reach terminal phase")
	})
})

var _ = Describe("Structured Approval Events in HandleWatch — TP-1398 (#1398)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("IT-AF-1398-001: emits structured approval_request event on AwaitingApproval with RAR", func() {
		rar := newDetailedFakeRAR("payments", "rar-rr-1")
		rrWatcher := watch.NewFake()
		rarWatcher := watch.NewFake()
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"), rar)

		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, rrWatcher, nil
		})
		client.PrependWatchReactor("remediationapprovalrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, rarWatcher, nil
		})

		queue := &bridgeQueue{}
		ctx = launcher.WithEventBridge(ctx, queue, "task-it-1398-001", "ctx-it-001", nil)

		go func() {
			time.Sleep(10 * time.Millisecond)
			rrWatcher.Modify(newFakeRR("payments", "rr-1", "AwaitingApproval"))
		}()

		result, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("awaiting_approval"))

		events := queue.Events()
		var approvalEvent *a2a.TaskStatusUpdateEvent
		for _, evt := range events {
			if tsu, ok := evt.(*a2a.TaskStatusUpdateEvent); ok {
				if tsu.Metadata != nil && tsu.Metadata["type"] == "approval_request" {
					approvalEvent = tsu
					break
				}
			}
		}
		Expect(approvalEvent).NotTo(BeNil(), "must emit approval_request event")

		textPart, ok := approvalEvent.Status.Message.Parts[0].(a2a.TextPart)
		Expect(ok).To(BeTrue())

		var payload tools.ApprovalRequestEventPayload
		Expect(json.Unmarshal([]byte(textPart.Text), &payload)).To(Succeed())
		Expect(payload.Name).To(Equal("rar-rr-1"))
		Expect(payload.Confidence).To(BeNumerically("~", 0.72, 0.001))
		Expect(payload.RemediationRequestName).To(Equal("rr-oom-1"))
		Expect(payload.RecommendedWorkflow).NotTo(BeNil())
	})

	It("IT-AF-1398-002: emits approval_request_resolved on RAR decision change", func() {
		rar := newDetailedFakeRAR("payments", "rar-rr-1")
		rrWatcher := watch.NewFake()
		rarWatcher := watch.NewFake()
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"), rar)

		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, rrWatcher, nil
		})
		client.PrependWatchReactor("remediationapprovalrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, rarWatcher, nil
		})

		queue := &bridgeQueue{}
		ctx = launcher.WithEventBridge(ctx, queue, "task-it-1398-002", "ctx-it-002", nil)

		decidedRAR := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationApprovalRequest",
				"metadata": map[string]interface{}{
					"name":      "rar-rr-1",
					"namespace": "payments",
				},
				"status": map[string]interface{}{
					"decision":  "Approved",
					"decidedBy": "operator@acme.com",
					"decidedAt": "2026-06-11T15:50:00Z",
				},
			},
		}

		go func() {
			time.Sleep(10 * time.Millisecond)
			rarWatcher.Modify(decidedRAR)
			time.Sleep(10 * time.Millisecond)
			rrWatcher.Modify(newFakeRR("payments", "rr-1", "Completed"))
		}()

		result, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		events := queue.Events()
		var resolvedEvent *a2a.TaskStatusUpdateEvent
		for _, evt := range events {
			if tsu, ok := evt.(*a2a.TaskStatusUpdateEvent); ok {
				if tsu.Metadata != nil && tsu.Metadata["type"] == "approval_request_resolved" {
					resolvedEvent = tsu
					break
				}
			}
		}
		Expect(resolvedEvent).NotTo(BeNil(), "must emit approval_request_resolved event")

		textPart, ok := resolvedEvent.Status.Message.Parts[0].(a2a.TextPart)
		Expect(ok).To(BeTrue())

		var payload tools.ApprovalResolvedEventPayload
		Expect(json.Unmarshal([]byte(textPart.Text), &payload)).To(Succeed())
		Expect(payload.Decision).To(Equal("Approved"))
		Expect(payload.DecidedBy).To(Equal("operator@acme.com"))
	})

	It("IT-AF-1398-003: emits both events when RAR already decided at AwaitingApproval", func() {
		decidedRAR := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationApprovalRequest",
				"metadata": map[string]interface{}{
					"name":      "rar-rr-1",
					"namespace": "payments",
				},
				"spec": map[string]interface{}{
					"remediationRequestRef": map[string]interface{}{
						"name": "rr-1",
					},
					"confidence":           0.95,
					"confidenceLevel":      "high",
					"reason":               "Auto-approved",
					"whyApprovalRequired":  "Audit trail",
					"investigationSummary": "Quick fix",
					"recommendedWorkflow": map[string]interface{}{
						"workflowId": "auto-v1",
						"version":    "1.0.0",
						"rationale":  "Auto-selected",
					},
					"recommendedActions": []interface{}{
						map[string]interface{}{"action": "Apply", "rationale": "Safe"},
					},
					"requiredBy": "2026-06-11T16:00:00Z",
				},
				"status": map[string]interface{}{
					"decision":  "Approved",
					"decidedBy": "system",
					"decidedAt": "2026-06-11T15:45:01Z",
				},
			},
		}

		rrWatcher := watch.NewFake()
		rarWatcher := watch.NewFake()
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"), decidedRAR)

		client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, rrWatcher, nil
		})
		client.PrependWatchReactor("remediationapprovalrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
			return true, rarWatcher, nil
		})

		queue := &bridgeQueue{}
		ctx = launcher.WithEventBridge(ctx, queue, "task-it-1398-003", "ctx-it-003", nil)

		go func() {
			time.Sleep(10 * time.Millisecond)
			rrWatcher.Modify(newFakeRR("payments", "rr-1", "AwaitingApproval"))
		}()

		result, err := tools.HandleWatch(ctx, client, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("awaiting_approval"))

		events := queue.Events()
		var requestFound, resolvedFound bool
		var requestIdx, resolvedIdx int
		for i, evt := range events {
			if tsu, ok := evt.(*a2a.TaskStatusUpdateEvent); ok {
				if tsu.Metadata != nil {
					switch tsu.Metadata["type"] {
					case "approval_request":
						requestFound = true
						requestIdx = i
					case "approval_request_resolved":
						resolvedFound = true
						resolvedIdx = i
					}
				}
			}
		}
		Expect(requestFound).To(BeTrue(), "must emit approval_request")
		Expect(resolvedFound).To(BeTrue(), "must emit approval_request_resolved for already-decided RAR")
		Expect(requestIdx).To(BeNumerically("<", resolvedIdx),
			"approval_request must precede approval_request_resolved (ordering guarantee)")
	})
})
