package controller_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/controller"
)

func TestController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var _ = Describe("RemediationOrchestrator Controller", func() {
	var (
		ctx        context.Context
		fakeClient client.Client
		scheme     *runtime.Scheme
		reconciler *controller.Reconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()

		// Register RemediationRequest CRD
		err := remediationv1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		reconciler = controller.NewReconciler(
			fakeClient,
			scheme,
			remediationorchestrator.DefaultConfig(),
		)
	})

	// BR-ORCH-025: Core reconciliation
	Describe("Reconcile", func() {
		Context("when RemediationRequest does not exist", func() {
			It("should return without error", func() {
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      "non-existent",
						Namespace: "default",
					},
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
			})
		})

		Context("when RemediationRequest exists with empty status", func() {
			var rr *remediationv1.RemediationRequest

			BeforeEach(func() {
				rr = &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr",
						Namespace: "default",
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
						SignalName:        "HighMemoryUsage",
						Severity:          "warning",
						Environment:       "production",
						Priority:          "P1",
						SignalType:        "prometheus",
						TargetType:        "kubernetes",
						TargetResource: remediationv1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
				}
				Expect(fakeClient.Create(ctx, rr)).To(Succeed())
			})

			It("should initialize status to Pending phase", func() {
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      rr.Name,
						Namespace: rr.Namespace,
					},
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Verify status was updated
				updated := &remediationv1.RemediationRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(rr), updated)).To(Succeed())
				Expect(updated.Status.OverallPhase).To(Equal("Pending"))
			})
		})

		Context("when RemediationRequest is in Pending phase", func() {
			var rr *remediationv1.RemediationRequest

			BeforeEach(func() {
				rr = &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr-pending",
						Namespace: "default",
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3",
						SignalName:        "HighMemoryUsage",
						Severity:          "warning",
						Environment:       "production",
						Priority:          "P1",
						SignalType:        "prometheus",
						TargetType:        "kubernetes",
						TargetResource: remediationv1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: "Pending",
					},
				}
				Expect(fakeClient.Create(ctx, rr)).To(Succeed())
			})

			It("should transition to Processing phase", func() {
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      rr.Name,
						Namespace: rr.Namespace,
					},
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Verify phase transition
				updated := &remediationv1.RemediationRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(rr), updated)).To(Succeed())
				Expect(updated.Status.OverallPhase).To(Equal("Processing"))
			})
		})

		// BR-ORCH-025: Terminal state handling
		Context("when RemediationRequest is in terminal state", func() {
			DescribeTable("should not requeue for terminal phases",
				func(terminalPhase string) {
					rr := &remediationv1.RemediationRequest{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-rr-terminal-" + terminalPhase,
							Namespace: "default",
						},
						Spec: remediationv1.RemediationRequestSpec{
							SignalFingerprint: "c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4",
							SignalName:        "TestSignal",
							Severity:          "warning",
							Environment:       "production",
							Priority:          "P2",
							SignalType:        "prometheus",
							TargetType:        "kubernetes",
							TargetResource: remediationv1.ResourceIdentifier{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: "default",
							},
						},
						Status: remediationv1.RemediationRequestStatus{
							OverallPhase: terminalPhase,
						},
					}
					Expect(fakeClient.Create(ctx, rr)).To(Succeed())

					result, err := reconciler.Reconcile(ctx, ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      rr.Name,
							Namespace: rr.Namespace,
						},
					})

					Expect(err).NotTo(HaveOccurred())
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(BeZero())
				},
				Entry("Completed phase", "Completed"),
				Entry("Failed phase", "Failed"),
				Entry("TimedOut phase", "TimedOut"),
				Entry("Skipped phase", "Skipped"),
			)
		})
	})

	// BR-ORCH-025: Reconciler construction
	Describe("NewReconciler", func() {
		It("should create a reconciler with provided client", func() {
			r := controller.NewReconciler(fakeClient, scheme, remediationorchestrator.DefaultConfig())
			Expect(r).NotTo(BeNil())
			Expect(r.Client).To(Equal(fakeClient))
		})

		It("should create a reconciler with provided scheme", func() {
			r := controller.NewReconciler(fakeClient, scheme, remediationorchestrator.DefaultConfig())
			Expect(r.Scheme).To(Equal(scheme))
		})

		It("should create a reconciler with provided config", func() {
			config := remediationorchestrator.OrchestratorConfig{
				MaxConcurrentReconciles: 5,
			}
			r := controller.NewReconciler(fakeClient, scheme, config)
			Expect(r.Config.MaxConcurrentReconciles).To(Equal(5))
		})
	})
})

