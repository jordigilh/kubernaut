package parentlifecycle_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/parentlifecycle"
)

var _ = Describe("Parent lifecycle gates", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
	})

	It("UT-RO-2582-006: bounds the create-before-parent-phase handoff [BR-ORCH-025]", func() {
		createdAt := time.Unix(100, 0)

		Expect(parentlifecycle.HandoffExpired(createdAt, createdAt.Add(2*time.Minute), 2*time.Minute)).To(BeTrue())
		Expect(parentlifecycle.HandoffExpired(createdAt, createdAt.Add(2*time.Minute-time.Nanosecond), 2*time.Minute)).To(BeFalse())
		Expect(parentlifecycle.HandoffExpired(time.Time{}, time.Now(), 2*time.Minute)).To(BeFalse())
	})

	Describe("RemediationRequestGate", func() {
		It("UT-RO-2582-005: allows work only in the persisted authorized phase [BR-ORCH-025, AC-4]", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rr-gate", Namespace: "default"},
				Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseExecuting},
			}
			reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).Build()

			result, err := parentlifecycle.RemediationRequestGate(ctx, reader, "default", "rr-gate", remediationv1.PhaseExecuting)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Decision).To(Equal(parentlifecycle.GateAllowed))
			Expect(result.ParentPhase).To(Equal(string(remediationv1.PhaseExecuting)))
		})

		It("UT-RO-2582-005: waits during the legitimate child-before-parent handoff [BR-ORCH-025]", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rr-wait", Namespace: "default"},
				Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseAnalyzing},
			}
			reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).Build()

			result, err := parentlifecycle.RemediationRequestGate(ctx, reader, "default", "rr-wait", remediationv1.PhaseExecuting)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Decision).To(Equal(parentlifecycle.GateWaiting))
		})

		It("UT-RO-2582-007: treats a missing or failed parent as cancellation [BR-COMMON-001, AC-4]", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()

			missing, err := parentlifecycle.RemediationRequestGate(ctx, reader, "default", "rr-missing", remediationv1.PhaseExecuting)
			Expect(err).NotTo(HaveOccurred())
			Expect(missing.Decision).To(Equal(parentlifecycle.GateCancelled))

			failedRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rr-failed", Namespace: "default"},
				Status:     remediationv1.RemediationRequestStatus{OverallPhase: remediationv1.PhaseFailed},
			}
			failedReader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(failedRR).Build()
			failed, err := parentlifecycle.RemediationRequestGate(ctx, failedReader, "default", "rr-failed", remediationv1.PhaseExecuting)
			Expect(err).NotTo(HaveOccurred())
			Expect(failed.Decision).To(Equal(parentlifecycle.GateCancelled))
		})

		It("UT-RO-2582-007: never authorizes work when the fresh parent read fails [AC-4, AC-6]", func() {
			readErr := fmt.Errorf("apiserver unavailable")
			reader := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
				Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
					return readErr
				},
			}).Build()

			_, err := parentlifecycle.RemediationRequestGate(ctx, reader, "default", "rr-read-error", remediationv1.PhaseExecuting)

			Expect(err).To(MatchError(ContainSubstring("read RemediationRequest")))
			Expect(errors.Is(err, readErr)).To(BeTrue())
		})
	})

	Describe("AIAnalysisGate", func() {
		It("UT-AI-2582-001: requires persisted Investigating before KA work [BR-AI-023, AC-4]", func() {
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{Name: "ai-gate", Namespace: "default"},
				Status:     aianalysisv1.AIAnalysisStatus{Phase: aianalysisv1.PhaseInvestigating},
			}
			reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(analysis).Build()

			result, err := parentlifecycle.AIAnalysisGate(ctx, reader, "default", "ai-gate", aianalysisv1.PhaseInvestigating)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Decision).To(Equal(parentlifecycle.GateAllowed))
		})

		It("UT-AI-2582-004: fails closed when AIAnalysis is gone [BR-AI-009, AC-4]", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()

			result, err := parentlifecycle.AIAnalysisGate(ctx, reader, "default", "ai-gone", aianalysisv1.PhaseInvestigating)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Decision).To(Equal(parentlifecycle.GateCancelled))
			Expect(apierrors.IsNotFound(result.Cause)).To(BeTrue())
		})
	})
})
