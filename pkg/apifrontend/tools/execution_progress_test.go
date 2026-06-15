package tools_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("Execution Progress Artifacts (#1403)", func() {

	Describe("BuildProgressSnapshot — UT-AF-1403-001..003", func() {
		DescribeTable("constructs correct payload",
			func(phase, rrName, startedAt, completedAt string, expectCompletedAt bool) {
				snapshot := tools.BuildProgressSnapshot(phase, rrName, startedAt, completedAt)
				Expect(snapshot).NotTo(BeNil())
				Expect(snapshot["type"]).To(Equal("execution_progress"))
				Expect(snapshot["schema_version"]).To(Equal("1.0"))
				Expect(snapshot["rr_name"]).To(Equal(rrName))
				Expect(snapshot["current_phase"]).To(Equal(phase))
				Expect(snapshot["started_at"]).To(Equal(startedAt))
				if expectCompletedAt {
					Expect(snapshot["completed_at"]).To(Equal(completedAt))
				} else {
					Expect(snapshot).NotTo(HaveKey("completed_at"))
				}
			},
			Entry("UT-AF-1403-001: non-terminal phase", "Executing", "rr-abc123", "2026-06-11T10:00:00Z", "", false),
			Entry("UT-AF-1403-002: terminal phase with completed_at", "Completed", "rr-abc123", "2026-06-11T10:00:00Z", "2026-06-11T10:05:00Z", true),
			Entry("UT-AF-1403-003: non-terminal phase omits completed_at", "Analyzing", "rr-def456", "2026-06-11T09:30:00Z", "", false),
		)
	})

	Describe("FetchStabilizationWindow (typed client) — UT-AF-1403-004..005", func() {

		newTypedEA := func(namespace, name string, stabilizationWindow time.Duration) *eav1alpha1.EffectivenessAssessment {
			return &eav1alpha1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Spec: eav1alpha1.EffectivenessAssessmentSpec{
					Config: eav1alpha1.EAConfig{
						StabilizationWindow: metav1.Duration{Duration: stabilizationWindow},
					},
				},
			}
		}

		eaScheme := func() *runtime.Scheme {
			s := runtime.NewScheme()
			_ = eav1alpha1.AddToScheme(s)
			return s
		}

		It("UT-AF-1403-004: returns stabilizationWindow from EA CRD", func() {
			ea := newTypedEA("payments", "ea-rr-abc123", 5*time.Minute)
			client := fake.NewClientBuilder().WithScheme(eaScheme()).WithObjects(ea).Build()

			result := tools.FetchStabilizationWindow(context.Background(), client, nil, "payments", "ea-rr-abc123")
			Expect(result).To(Equal("5m0s"))
		})

		It("UT-AF-1403-005: returns empty string when EA not found (graceful fallback)", func() {
			client := fake.NewClientBuilder().WithScheme(eaScheme()).Build()

			result := tools.FetchStabilizationWindow(context.Background(), client, nil, "payments", "ea-nonexistent")
			Expect(result).To(BeEmpty())
		})

		It("UT-AF-1403-005b: returns empty string when reader is nil", func() {
			result := tools.FetchStabilizationWindow(context.Background(), nil, nil, "payments", "ea-rr-1")
			Expect(result).To(BeEmpty())
		})
	})

	Describe("Schema validation — UT-AF-1403-006", func() {
		It("UT-AF-1403-006: execution_progress schema validates correct payload", func() {
			payload := map[string]any{
				"type":           "execution_progress",
				"schema_version": "1.0",
				"rr_name":        "rr-abc123",
				"current_phase":  "Executing",
				"started_at":     "2026-06-11T10:00:00Z",
			}
			err := launcher.ValidatePayload("execution_progress", payload)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("EmitArtifactSafe — UT-AF-1403-007", func() {
		It("UT-AF-1403-007: EmitArtifactSafe is nil-safe (no bridge in context)", func() {
			err := launcher.EmitArtifactSafe(context.Background(), map[string]any{"type": "execution_progress"}, "Progress: Executing", map[string]any{"type": "execution_progress"})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("HandleWatch emits progress artifacts — UT-AF-1403-008..010", func() {
		var ctx context.Context

		BeforeEach(func() {
			ctx = context.Background()
		})

		It("UT-AF-1403-008: emits TaskArtifactUpdateEvent on phase transition", func() {
			fakeWatcher := watch.NewFake()
			client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Pending"))
			client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
				return true, fakeWatcher, nil
			})

			queue := &bridgeQueue{}
			ctx = launcher.WithEventBridge(ctx, queue, "task-1403-008", "ctx-1403-008", nil)

			go func() {
				defer fakeWatcher.Stop()
				time.Sleep(10 * time.Millisecond)
				fakeWatcher.Modify(newFakeRR("payments", "rr-1", "Executing"))
				time.Sleep(10 * time.Millisecond)
				fakeWatcher.Modify(newFakeRR("payments", "rr-1", "Completed"))
			}()

			result, err := tools.HandleWatch(ctx, client, nil, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("completed"))

			events := queue.Events()
			var artifactEvents []*a2a.TaskArtifactUpdateEvent
			for _, evt := range events {
				if art, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
					if art.Artifact != nil && art.Artifact.Metadata != nil {
						if art.Artifact.Metadata["type"] == "execution_progress" {
							artifactEvents = append(artifactEvents, art)
						}
					}
				}
			}
			Expect(artifactEvents).NotTo(BeEmpty(), "expected at least one execution_progress artifact event")

			firstArt := artifactEvents[0]
			Expect(firstArt.Artifact.Parts).To(HaveLen(2))

			dataPart, ok := firstArt.Artifact.Parts[0].(a2a.DataPart)
			Expect(ok).To(BeTrue(), "Part[0] should be DataPart")
			Expect(dataPart.Data["type"]).To(Equal("execution_progress"))
			Expect(dataPart.Data["rr_name"]).To(Equal("rr-1"))
		})

		It("UT-AF-1403-009: includes stabilization_window in metadata on Verifying phase", func() {
			fakeRR := newFakeRR("payments", "rr-1", "Executing")

			dynScheme := runtime.NewScheme()
			dynScheme.AddKnownTypeWithName(
				schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "RemediationRequestList"},
				&unstructured.UnstructuredList{},
			)
			dynScheme.AddKnownTypeWithName(
				schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "RemediationApprovalRequestList"},
				&unstructured.UnstructuredList{},
			)
			dynClient := dynamicfake.NewSimpleDynamicClient(dynScheme, fakeRR)

			fakeWatcher := watch.NewFake()
			dynClient.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
				return true, fakeWatcher, nil
			})

			typedEA := &eav1alpha1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{Name: "ea-rr-1", Namespace: "payments"},
				Spec: eav1alpha1.EffectivenessAssessmentSpec{
					Config: eav1alpha1.EAConfig{
						StabilizationWindow: metav1.Duration{Duration: 5 * time.Minute},
					},
				},
			}
			eaScheme := runtime.NewScheme()
			_ = eav1alpha1.AddToScheme(eaScheme)
			typedClient := fake.NewClientBuilder().WithScheme(eaScheme).WithObjects(typedEA).Build()

			queue := &bridgeQueue{}
			ctx = launcher.WithEventBridge(ctx, queue, "task-1403-009", "ctx-1403-009", nil)

			verifyingRR := newFakeRR("payments", "rr-1", "Verifying")
			completedRR := newFakeRR("payments", "rr-1", "Completed")

			go func() {
				defer fakeWatcher.Stop()
				time.Sleep(10 * time.Millisecond)
				fakeWatcher.Modify(verifyingRR)
				time.Sleep(10 * time.Millisecond)
				fakeWatcher.Modify(completedRR)
			}()

			result, err := tools.HandleWatch(ctx, dynClient, typedClient, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("completed"))

			events := queue.Events()
			var verifyingArtifact *a2a.TaskArtifactUpdateEvent
			for _, evt := range events {
				if art, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
					if art.Artifact != nil && art.Artifact.Metadata != nil && art.Artifact.Metadata["type"] == "execution_progress" {
						for _, p := range art.Artifact.Parts {
							if dp, dpOK := p.(a2a.DataPart); dpOK {
								if dp.Data["current_phase"] == "Verifying" {
									verifyingArtifact = art
								}
							}
						}
					}
				}
			}
			Expect(verifyingArtifact).NotTo(BeNil(), "expected a Verifying progress artifact")
			Expect(verifyingArtifact.Artifact.Metadata["stabilization_window"]).To(Equal("5m0s"))
		})

		It("UT-AF-1403-010: omits stabilization_window when EA ref absent", func() {
			fakeWatcher := watch.NewFake()
			client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"))
			client.PrependWatchReactor("remediationrequests", func(action k8stesting.Action) (bool, watch.Interface, error) {
				return true, fakeWatcher, nil
			})

			queue := &bridgeQueue{}
			ctx = launcher.WithEventBridge(ctx, queue, "task-1403-010", "ctx-1403-010", nil)

			verifyingRR := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kubernaut.ai/v1alpha1",
					"kind":       "RemediationRequest",
					"metadata": map[string]interface{}{
						"name":      "rr-1",
						"namespace": "payments",
					},
					"status": map[string]interface{}{
						"overallPhase": "Verifying",
					},
				},
			}

			completedRR := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kubernaut.ai/v1alpha1",
					"kind":       "RemediationRequest",
					"metadata": map[string]interface{}{
						"name":      "rr-1",
						"namespace": "payments",
					},
					"status": map[string]interface{}{
						"overallPhase": "Completed",
					},
				},
			}

			go func() {
				defer fakeWatcher.Stop()
				time.Sleep(10 * time.Millisecond)
				fakeWatcher.Modify(verifyingRR)
				time.Sleep(10 * time.Millisecond)
				fakeWatcher.Modify(completedRR)
			}()

			result, err := tools.HandleWatch(ctx, client, nil, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("completed"))

			events := queue.Events()
			for _, evt := range events {
				if art, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
					if art.Artifact != nil && art.Artifact.Metadata != nil && art.Artifact.Metadata["type"] == "execution_progress" {
						Expect(art.Artifact.Metadata).NotTo(HaveKey("stabilization_window"),
							"stabilization_window should not be present when typedClient is nil")
					}
				}
			}
		})
	})

	Describe("Progress artifact DataPart JSON structure — UT-AF-1403-011", func() {
		It("UT-AF-1403-011: DataPart payload is JSON-serializable with expected fields", func() {
			snapshot := tools.BuildProgressSnapshot("Verifying", "rr-xyz789", "2026-06-11T10:00:00Z", "")
			data, err := json.Marshal(snapshot)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]any
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())
			Expect(parsed["type"]).To(Equal("execution_progress"))
			Expect(parsed["schema_version"]).To(Equal("1.0"))
			Expect(parsed["rr_name"]).To(Equal("rr-xyz789"))
			Expect(parsed["current_phase"]).To(Equal("Verifying"))
		})
	})
})
