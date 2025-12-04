package reconciler

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/signalprocessing"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// BR-SP-051: Phase State Machine
// BR-SP-052: K8s Enrichment
// BR-SP-053: Environment Classification
// BR-SP-054: Priority Classification
// BR-SP-100: OwnerChain Traversal
// BR-SP-101: DetectedLabels Auto-Detection
// BR-SP-102: CustomLabels Rego Extraction
var _ = Describe("BR-SP-051 to BR-SP-102: SignalProcessing Reconciler", func() {
	var (
		ctx        context.Context
		scheme     *runtime.Scheme
		reconciler *signalprocessing.SignalProcessingReconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(signalprocessingv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	})

	// ========================================
	// BR-SP-051: Phase State Machine
	// ========================================
	Describe("Phase State Machine", func() {
		It("should initialize pending SignalProcessing to enriching phase", func() {
			sp := createSignalProcessing("test-sp", "default", "")
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(sp).
				WithStatusSubresource(sp).
				Build()

			reconciler = signalprocessing.NewReconcilerWithDependencies(fakeClient, scheme, ctrl.Log.WithName("test"))

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "test-sp",
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			// Verify status was updated
			updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test-sp"}, updatedSP)).To(Succeed())
			Expect(updatedSP.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseEnriching))
			Expect(updatedSP.Status.StartTime).ToNot(BeNil())
		})

		It("should not requeue completed SignalProcessing", func() {
			sp := createSignalProcessing("test-sp", "default", "completed")
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(sp).
				WithStatusSubresource(sp).
				Build()

			reconciler = signalprocessing.NewReconcilerWithDependencies(fakeClient, scheme, ctrl.Log.WithName("test"))

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "test-sp",
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})

		It("should not requeue failed SignalProcessing", func() {
			sp := createSignalProcessing("test-sp", "default", "failed")
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(sp).
				WithStatusSubresource(sp).
				Build()

			reconciler = signalprocessing.NewReconcilerWithDependencies(fakeClient, scheme, ctrl.Log.WithName("test"))

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "test-sp",
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})
	})

	// ========================================
	// BR-SP-052: K8s Enrichment
	// ========================================
	Describe("K8s Enrichment Phase", func() {
		It("should enrich SignalProcessing with K8s context", func() {
			sp := createSignalProcessing("test-sp", "default", "enriching")
			pod := createTestPod("test-pod", "default")
			ns := createTestNamespace("default", map[string]string{"environment": "production"})

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(sp, pod, ns).
				WithStatusSubresource(sp).
				Build()

			reconciler = signalprocessing.NewReconcilerWithDependencies(fakeClient, scheme, ctrl.Log.WithName("test"))

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "test-sp",
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			// Verify enrichment results
			updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test-sp"}, updatedSP)).To(Succeed())
			Expect(updatedSP.Status.EnrichmentResults).ToNot(BeNil())
			Expect(updatedSP.Status.EnrichmentResults.KubernetesContext).ToNot(BeNil())
			Expect(updatedSP.Status.EnrichmentResults.KubernetesContext.Namespace).To(Equal("default"))
		})
	})

	// ========================================
	// BR-SP-053: Environment Classification
	// ========================================
	Describe("Environment Classification Phase", func() {
		It("should classify environment from namespace labels", func() {
			sp := createSignalProcessingWithEnrichment("test-sp", "default", "classifying")
			ns := createTestNamespace("default", map[string]string{"environment": "production"})

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(sp, ns).
				WithStatusSubresource(sp).
				Build()

			reconciler = signalprocessing.NewReconcilerWithDependencies(fakeClient, scheme, ctrl.Log.WithName("test"))

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "test-sp",
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			// Verify classification
			updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test-sp"}, updatedSP)).To(Succeed())
			Expect(updatedSP.Status.ClassificationResults).ToNot(BeNil())
			Expect(updatedSP.Status.ClassificationResults.Environment).To(Equal("production"))
		})
	})

	// ========================================
	// BR-SP-054: Priority Classification
	// ========================================
	Describe("Priority Classification Phase", func() {
		It("should classify priority based on signal severity", func() {
			sp := createSignalProcessingWithEnrichment("test-sp", "default", "classifying")
			sp.Spec.Severity = "critical"

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(sp).
				WithStatusSubresource(sp).
				Build()

			reconciler = signalprocessing.NewReconcilerWithDependencies(fakeClient, scheme, ctrl.Log.WithName("test"))

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "test-sp",
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			// Verify priority
			updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test-sp"}, updatedSP)).To(Succeed())
			Expect(updatedSP.Status.ClassificationResults.Priority).To(Equal("P1"))
		})
	})

	// ========================================
	// BR-SP-100: OwnerChain Traversal
	// ========================================
	Describe("OwnerChain Traversal", func() {
		It("should build owner chain for pods with ownership", func() {
			sp := createSignalProcessingWithEnrichment("test-sp", "default", "enriching")
			pod := createPodWithOwner("test-pod", "default", "ReplicaSet", "test-rs")
			rs := createReplicaSetWithOwner("test-rs", "default", "Deployment", "test-deploy")
			deploy := createDeployment("test-deploy", "default")

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(sp, pod, rs, deploy).
				WithStatusSubresource(sp).
				Build()

			reconciler = signalprocessing.NewReconcilerWithDependencies(fakeClient, scheme, ctrl.Log.WithName("test"))

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "test-sp",
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			// Verify owner chain
			updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test-sp"}, updatedSP)).To(Succeed())
			Expect(updatedSP.Status.EnrichmentResults.OwnerChain).To(HaveLen(3))
			Expect(updatedSP.Status.EnrichmentResults.OwnerChain[0].Kind).To(Equal("Pod"))
			Expect(updatedSP.Status.EnrichmentResults.OwnerChain[1].Kind).To(Equal("ReplicaSet"))
			Expect(updatedSP.Status.EnrichmentResults.OwnerChain[2].Kind).To(Equal("Deployment"))
		})
	})

	// ========================================
	// BR-SP-101: DetectedLabels Auto-Detection
	// ========================================
	Describe("DetectedLabels Auto-Detection", func() {
		It("should detect GitOps management from annotations", func() {
			sp := createSignalProcessingWithEnrichment("test-sp", "default", "enriching")
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-deploy",
					Namespace:   "default",
					Annotations: map[string]string{"argocd.argoproj.io/instance": "test-app"},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(sp, deploy).
				WithStatusSubresource(sp).
				Build()

			reconciler = signalprocessing.NewReconcilerWithDependencies(fakeClient, scheme, ctrl.Log.WithName("test"))

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "test-sp",
				},
			})

			Expect(err).ToNot(HaveOccurred())

			// Verify detected labels
			updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test-sp"}, updatedSP)).To(Succeed())
			Expect(updatedSP.Status.EnrichmentResults.DetectedLabels).ToNot(BeNil())
			Expect(updatedSP.Status.EnrichmentResults.DetectedLabels.GitOpsManaged).To(BeTrue())
			Expect(updatedSP.Status.EnrichmentResults.DetectedLabels.GitOpsTool).To(Equal("argocd"))
		})
	})

	// ========================================
	// Full Workflow Test
	// ========================================
	Describe("Full Workflow", func() {
		It("should process SignalProcessing through all phases to completion", func() {
			sp := createSignalProcessing("test-sp", "default", "")
			sp.Spec.SignalName = "pod-crash-loop"
			sp.Spec.Severity = "critical"
			pod := createTestPod("test-pod", "default")
			ns := createTestNamespace("default", map[string]string{"environment": "production", "team": "payments"})

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(sp, pod, ns).
				WithStatusSubresource(sp).
				Build()

			reconciler = signalprocessing.NewReconcilerWithDependencies(fakeClient, scheme, ctrl.Log.WithName("test"))

			// Run reconciliation multiple times to progress through phases
			for i := 0; i < 5; i++ {
				result, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "default",
						Name:      "test-sp",
					},
				})
				Expect(err).ToNot(HaveOccurred())

				updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
				Expect(fakeClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test-sp"}, updatedSP)).To(Succeed())

				if updatedSP.Status.Phase == signalprocessingv1alpha1.PhaseCompleted || !result.Requeue {
					break
				}
			}

			// Verify final state
			finalSP := &signalprocessingv1alpha1.SignalProcessing{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test-sp"}, finalSP)).To(Succeed())
			Expect(finalSP.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
			Expect(finalSP.Status.EnrichmentResults).ToNot(BeNil())
			Expect(finalSP.Status.ClassificationResults).ToNot(BeNil())
			Expect(finalSP.Status.ClassificationResults.Environment).To(Equal("production"))
			Expect(finalSP.Status.ClassificationResults.Priority).To(Equal("P1"))
		})
	})

	// ========================================
	// Error Handling
	// ========================================
	Describe("Error Handling", func() {
		It("should handle not found resource gracefully", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			reconciler = signalprocessing.NewReconcilerWithDependencies(fakeClient, scheme, ctrl.Log.WithName("test"))

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "non-existent",
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})
	})
})

// Helper functions
func createSignalProcessing(name, namespace string, phase string) *signalprocessingv1alpha1.SignalProcessing {
	sp := &signalprocessingv1alpha1.SignalProcessing{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: signalprocessingv1alpha1.SignalProcessingSpec{
			SignalFingerprint: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			SignalName:        "test-signal",
			Severity:          "warning",
			Environment:       "development",
			Priority:          "P2",
			SignalType:        "prometheus",
			TargetType:        "kubernetes",
			TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
				Namespace: namespace,
				Kind:      "Pod",
				Name:      "test-pod",
			},
			ReceivedTime: metav1.Now(),
			Deduplication: sharedtypes.DeduplicationInfo{
				CorrelationID: "test-correlation-id",
			},
		},
	}
	if phase != "" {
		sp.Status.Phase = signalprocessingv1alpha1.SignalProcessingPhase(phase)
	}
	return sp
}

func createSignalProcessingWithEnrichment(name, namespace, phase string) *signalprocessingv1alpha1.SignalProcessing {
	sp := createSignalProcessing(name, namespace, phase)
	sp.Status.EnrichmentResults = &sharedtypes.EnrichmentResults{
		KubernetesContext: &sharedtypes.KubernetesContext{
			Namespace: namespace,
			PodDetails: &sharedtypes.PodDetails{
				Name: "test-pod",
			},
		},
	}
	return sp
}

func createTestPod(name, namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
}

func createTestNamespace(name string, labels map[string]string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
}

func createPodWithOwner(name, namespace, ownerKind, ownerName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       ownerKind,
					Name:       ownerName,
					Controller: boolPtr(true),
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
}

func createReplicaSetWithOwner(name, namespace, ownerKind, ownerName string) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       ownerKind,
					Name:       ownerName,
					Controller: boolPtr(true),
				},
			},
		},
	}
}

func createDeployment(name, namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func boolPtr(b bool) *bool {
	return &b
}

var _ = BeforeSuite(func() {
	// Set up test timeout
	SetDefaultEventuallyTimeout(30 * time.Second)
	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
})
