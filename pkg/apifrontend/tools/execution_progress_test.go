package tools_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	executionProgress = "execution_progress"
)

var _ = Describe("Execution Progress Artifacts (#1403)", func() {

	Describe("BuildProgressSnapshot — UT-AF-1403-001..003", func() {
		DescribeTable("constructs correct payload",
			func(phase, rrName, startedAt, completedAt string, expectCompletedAt bool) {
				snapshot := tools.BuildProgressSnapshot(phase, rrName, startedAt, completedAt, "")
				Expect(snapshot).NotTo(BeNil())
				Expect(snapshot["type"]).To(Equal(executionProgress))
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

	Describe("BuildProgressSnapshot cluster attribution — UT-AF-1409-007/008", func() {
		It("UT-AF-1409-007: AU-3 — carries cluster_id when the RR being watched has one", func() {
			snapshot := tools.BuildProgressSnapshot("Executing", "rr-fleet-001", "2026-06-11T10:00:00Z", "", "cluster-fleet-b")
			Expect(snapshot).To(HaveKeyWithValue("cluster_id", "cluster-fleet-b"),
				"AU-3: execution_progress must carry cluster attribution for Console multi-cluster context")
		})

		It("UT-AF-1409-008: AU-3 — omits cluster_id entirely for local-hub RRs (no false attribution)", func() {
			snapshot := tools.BuildProgressSnapshot("Executing", "rr-local-001", "2026-06-11T10:00:00Z", "", "")
			Expect(snapshot).NotTo(HaveKey("cluster_id"),
				"AU-3: local-hub RRs must not carry an empty-string cluster_id (false attribution noise)")
		})
	})

	Describe("FetchStabilizationWindow (typed client) — UT-AF-1403-004..005", func() {

		It("UT-AF-1403-004: returns stabilization window when EA exists", func() {
			ea := &eav1alpha1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{Name: "ea-rr-1", Namespace: "payments"},
				Spec: eav1alpha1.EffectivenessAssessmentSpec{
					Config: eav1alpha1.EAConfig{
						StabilizationWindow: metav1.Duration{Duration: 5 * time.Minute},
					},
				},
			}
			fc := fake.NewClientBuilder().WithScheme(watchTestScheme()).WithObjects(ea).Build()

			timing := tools.FetchEATimingMetadata(context.Background(), fc, nil, "payments", "ea-rr-1")
			Expect(timing.StabilizationWindow).To(Equal("5m0s"))
		})

		It("UT-AF-1403-005: returns empty stabilization window when EA absent", func() {
			fc := fake.NewClientBuilder().WithScheme(watchTestScheme()).Build()

			timing := tools.FetchEATimingMetadata(context.Background(), fc, nil, "payments", "ea-nonexistent")
			Expect(timing.StabilizationWindow).To(BeEmpty())
		})
	})

	Describe("FetchEATimingMetadata — validity_deadline (#1426)", func() {

		It("UT-AF-1426-004: returns validity_deadline when EA has it", func() {
			deadline := metav1.NewTime(time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC))
			ea := &eav1alpha1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{Name: "ea-rr-deadline", Namespace: "kubernaut-system"},
				Spec: eav1alpha1.EffectivenessAssessmentSpec{
					Config: eav1alpha1.EAConfig{
						StabilizationWindow: metav1.Duration{Duration: 3 * time.Minute},
					},
				},
				Status: eav1alpha1.EffectivenessAssessmentStatus{
					ValidityDeadline: &deadline,
				},
			}
			fc := fake.NewClientBuilder().WithScheme(watchTestScheme()).WithObjects(ea).WithStatusSubresource(ea).Build()

			timing := tools.FetchEATimingMetadata(context.Background(), fc, nil, "kubernaut-system", "ea-rr-deadline")
			Expect(timing.StabilizationWindow).To(Equal("3m0s"))
			Expect(timing.ValidityDeadline).To(Equal("2026-06-15T10:30:00Z"))
		})

		It("UT-AF-1426-005: returns empty validity_deadline when EA has no deadline", func() {
			ea := &eav1alpha1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{Name: "ea-rr-nodeadline", Namespace: "kubernaut-system"},
				Spec: eav1alpha1.EffectivenessAssessmentSpec{
					Config: eav1alpha1.EAConfig{
						StabilizationWindow: metav1.Duration{Duration: 2 * time.Minute},
					},
				},
			}
			fc := fake.NewClientBuilder().WithScheme(watchTestScheme()).WithObjects(ea).Build()

			timing := tools.FetchEATimingMetadata(context.Background(), fc, nil, "kubernaut-system", "ea-rr-nodeadline")
			Expect(timing.StabilizationWindow).To(Equal("2m0s"))
			Expect(timing.ValidityDeadline).To(BeEmpty())
		})
	})

	Describe("FetchEATimingMetadata — HandleWatch Verifying integration (#1426)", func() {

		It("UT-AF-1426-007: Verifying status event has stabilization_window, started_at, validity_deadline from EA", func() {
			deadline := metav1.NewTime(time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC))
			ea := &eav1alpha1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{Name: "ea-rr-1", Namespace: "payments"},
				Spec: eav1alpha1.EffectivenessAssessmentSpec{
					Config: eav1alpha1.EAConfig{
						StabilizationWindow: metav1.Duration{Duration: 5 * time.Minute},
					},
				},
				Status: eav1alpha1.EffectivenessAssessmentStatus{
					ValidityDeadline: &deadline,
				},
			}
			fc := fake.NewClientBuilder().WithScheme(watchTestScheme()).WithObjects(ea).WithStatusSubresource(ea).Build()

			timing := tools.FetchEATimingMetadata(context.Background(), fc, nil, "payments", "ea-rr-1")
			Expect(timing.StabilizationWindow).To(Equal("5m0s"))
			Expect(timing.ValidityDeadline).To(Equal("2026-06-15T10:30:00Z"))

			statusMeta := map[string]any{"type": launcher.MetaTypeStatus}
			if timing.StabilizationWindow != "" {
				statusMeta["stabilization_window"] = timing.StabilizationWindow
				statusMeta["started_at"] = time.Now().UTC().Format(time.RFC3339)
			}
			if timing.ValidityDeadline != "" {
				statusMeta["validity_deadline"] = timing.ValidityDeadline
			}

			Expect(statusMeta["stabilization_window"]).To(Equal("5m0s"))
			Expect(statusMeta).To(HaveKey("started_at"))
			Expect(statusMeta["validity_deadline"]).To(Equal("2026-06-15T10:30:00Z"))
		})
	})

	Describe("HandleWatch emits progress artifacts — UT-AF-1403-008..010", func() {
		var ctx context.Context

		BeforeEach(func() {
			ctx = context.Background()
		})

		It("UT-AF-1403-008: emits TaskArtifactUpdateEvent on phase transition", func() {
			rr := newTypedRR("payments", "rr-1", "Pending")
			wc := newWatchClient(rr)

			queue := &bridgeQueue{}
			ctx = launcher.WithEventBridge(ctx, queue, "task-1403-008", "ctx-1403-008", nil)

			go func() {
				time.Sleep(50 * time.Millisecond)
				updateRRPhase(ctx, wc, "rr-1", "Executing")
				time.Sleep(50 * time.Millisecond)
				updateRRTerminal(ctx, wc, "rr-1", "success")
			}()

			result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("completed"))

			events := queue.Events()
			var artifactEvents []*a2a.TaskArtifactUpdateEvent
			for _, evt := range events {
				if art, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
					if art.Artifact != nil && art.Artifact.Metadata != nil {
						if art.Artifact.Metadata["type"] == executionProgress {
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
			Expect(dataPart.Data["type"]).To(Equal(executionProgress))
			Expect(dataPart.Data["rr_name"]).To(Equal("rr-1"))
		})

		It("UT-AF-1403-009: includes stabilization_window in metadata on Verifying phase", func() {
			rr := newTypedRR("payments", "rr-1", "Executing")
			typedEA := &eav1alpha1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{Name: "ea-rr-1", Namespace: "payments"},
				Spec: eav1alpha1.EffectivenessAssessmentSpec{
					Config: eav1alpha1.EAConfig{
						StabilizationWindow: metav1.Duration{Duration: 5 * time.Minute},
					},
				},
			}
			wc := newWatchClient(rr, typedEA)

			queue := &bridgeQueue{}
			ctx = launcher.WithEventBridge(ctx, queue, "task-1403-009", "ctx-1403-009", nil)

			go func() {
				time.Sleep(50 * time.Millisecond)
				updateRRPhase(ctx, wc, "rr-1", "Verifying")
				time.Sleep(50 * time.Millisecond)
				updateRRTerminal(ctx, wc, "rr-1", "success")
			}()

			result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("completed"))

			events := queue.Events()
			var verifyingArtifact *a2a.TaskArtifactUpdateEvent
			for _, evt := range events {
				if art, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
					if art.Artifact != nil && art.Artifact.Metadata != nil && art.Artifact.Metadata["type"] == executionProgress {
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

		It("UT-AF-1426-006: Verifying phase emits status event with stabilization_window + started_at + validity_deadline", func() {
			rr := newTypedRR("payments", "rr-1", "Executing")
			deadline := metav1.NewTime(time.Date(2026, 6, 15, 12, 30, 0, 0, time.UTC))
			typedEA := &eav1alpha1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{Name: "ea-rr-1", Namespace: "payments"},
				Spec: eav1alpha1.EffectivenessAssessmentSpec{
					Config: eav1alpha1.EAConfig{
						StabilizationWindow: metav1.Duration{Duration: 5 * time.Minute},
					},
				},
				Status: eav1alpha1.EffectivenessAssessmentStatus{
					ValidityDeadline: &deadline,
				},
			}
			wc := newWatchClient(rr, typedEA)

			queue := &bridgeQueue{}
			ctx = launcher.WithEventBridge(ctx, queue, "task-1426-006", "ctx-1426-006", nil)

			go func() {
				time.Sleep(50 * time.Millisecond)
				updateRRPhase(ctx, wc, "rr-1", "Verifying")
				time.Sleep(50 * time.Millisecond)
				updateRRTerminal(ctx, wc, "rr-1", "success")
			}()

			result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("completed"))

			events := queue.Events()
			var verifyingStatus *a2a.TaskStatusUpdateEvent
			for _, evt := range events {
				if se, ok := evt.(*a2a.TaskStatusUpdateEvent); ok {
					if se.Metadata != nil && se.Metadata["type"] == "status" {
						if se.Status.Message != nil {
							for _, p := range se.Status.Message.Parts {
								if tp, tpOK := p.(a2a.TextPart); tpOK {
									if tp.Text == "Remediation phase: Verifying\n" {
										verifyingStatus = se
									}
								}
							}
						}
					}
				}
			}
			Expect(verifyingStatus).NotTo(BeNil(), "expected a Verifying status event with metadata")
			Expect(verifyingStatus.Metadata["stabilization_window"]).To(Equal("5m0s"))
			Expect(verifyingStatus.Metadata).To(HaveKey("started_at"))
			Expect(verifyingStatus.Metadata["validity_deadline"]).To(Equal("2026-06-15T12:30:00Z"))
		})

		It("UT-AF-1403-010: omits stabilization_window when EA ref absent", func() {
			rr := newTypedRR("payments", "rr-1", "Executing")
			wc := newWatchClient(rr)

			queue := &bridgeQueue{}
			ctx = launcher.WithEventBridge(ctx, queue, "task-1403-010", "ctx-1403-010", nil)

			go func() {
				time.Sleep(50 * time.Millisecond)
				updateRRPhase(ctx, wc, "rr-1", "Verifying")
				time.Sleep(50 * time.Millisecond)
				updateRRTerminal(ctx, wc, "rr-1", "success")
			}()

			result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("completed"))

			events := queue.Events()
			for _, evt := range events {
				if art, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
					if art.Artifact != nil && art.Artifact.Metadata != nil && art.Artifact.Metadata["type"] == executionProgress {
						Expect(art.Artifact.Metadata).NotTo(HaveKey("stabilization_window"),
							"stabilization_window should not be present when EA does not exist")
					}
				}
			}
		})
	})

	Describe("Progress artifact DataPart JSON structure — UT-AF-1403-011", func() {
		It("UT-AF-1403-011: DataPart payload is JSON-serializable with expected fields", func() {
			snapshot := tools.BuildProgressSnapshot("Verifying", "rr-xyz789", "2026-06-11T10:00:00Z", "", "")
			data, err := json.Marshal(snapshot)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]any
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())
			Expect(parsed["type"]).To(Equal(executionProgress))
			Expect(parsed["schema_version"]).To(Equal("1.0"))
			Expect(parsed["rr_name"]).To(Equal("rr-xyz789"))
			Expect(parsed["current_phase"]).To(Equal("Verifying"))
		})
	})
})
