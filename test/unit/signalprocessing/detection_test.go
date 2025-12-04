package signalprocessing

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/detection"
)

// BR-SP-101: DetectedLabels Auto-Detection
// DD-WORKFLOW-001 v2.2: 7 auto-detected cluster characteristics
var _ = Describe("BR-SP-101: DetectedLabels Detector", func() {
	var (
		ctx      context.Context
		scheme   *runtime.Scheme
		detector *detection.LabelDetector
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
		Expect(autoscalingv2.AddToScheme(scheme)).To(Succeed())
		Expect(policyv1.AddToScheme(scheme)).To(Succeed())
		Expect(networkingv1.AddToScheme(scheme)).To(Succeed())
	})

	// ========================================
	// TC-DL-001: GitOps Detection (ArgoCD)
	// ========================================
	Describe("GitOps Detection", func() {
		DescribeTable("should detect GitOps management",
			func(annotations map[string]string, objLabels map[string]string, expectedManaged bool, expectedTool string) {
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "web-app",
						Namespace:   "default",
						Annotations: annotations,
						Labels:      objLabels,
					},
				}
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithRuntimeObjects(deploy).
					Build()

				detector = detection.NewLabelDetector(fakeClient, ctrl.Log.WithName("test"))

				k8sCtx := &sharedtypes.KubernetesContext{
					Namespace: "default",
					DeploymentDetails: &sharedtypes.DeploymentDetails{
						Name: "web-app",
					},
				}

				result := detector.DetectLabels(ctx, k8sCtx)

				Expect(result.GitOpsManaged).To(Equal(expectedManaged))
				Expect(result.GitOpsTool).To(Equal(expectedTool))
			},
			Entry("TC-DL-001a: ArgoCD managed deployment",
				map[string]string{"argocd.argoproj.io/instance": "web-app"}, nil, true, "argocd"),
			Entry("TC-DL-002: Flux managed deployment",
				nil, map[string]string{"fluxcd.io/sync-gc-mark": "sha256:abc123"}, true, "flux"),
			Entry("TC-DL-001b: No GitOps management",
				nil, nil, false, ""),
		)
	})

	// ========================================
	// TC-DL-003: PDB Protection Detection
	// ========================================
	Describe("PDB Protection Detection", func() {
		It("TC-DL-003: should detect PDB exists for deployment", func() {
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-app",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "web"},
					},
				},
			}
			pdb := &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-pdb",
					Namespace: "default",
				},
				Spec: policyv1.PodDisruptionBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "web"},
					},
					MinAvailable: &intstr.IntOrString{IntVal: 1},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(deploy, pdb).
				Build()

			detector = detection.NewLabelDetector(fakeClient, ctrl.Log.WithName("test"))

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "default",
				DeploymentDetails: &sharedtypes.DeploymentDetails{
					Name:   "web-app",
					Labels: map[string]string{"app": "web"},
				},
			}

			labels := detector.DetectLabels(ctx, k8sCtx)
			Expect(labels.PDBProtected).To(BeTrue())
		})

		It("should return false when no PDB exists", func() {
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-app",
					Namespace: "default",
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(deploy).
				Build()

			detector = detection.NewLabelDetector(fakeClient, ctrl.Log.WithName("test"))

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "default",
				DeploymentDetails: &sharedtypes.DeploymentDetails{
					Name: "web-app",
				},
			}

			labels := detector.DetectLabels(ctx, k8sCtx)
			Expect(labels.PDBProtected).To(BeFalse())
			Expect(labels.FailedDetections).To(BeEmpty()) // No query failure, just no PDB
		})
	})

	// ========================================
	// TC-DL-004: HPA Detection
	// ========================================
	Describe("HPA Detection", func() {
		It("TC-DL-004: should detect HPA exists for deployment", func() {
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-app",
					Namespace: "default",
				},
			}
			hpa := &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-hpa",
					Namespace: "default",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind: "Deployment",
						Name: "web-app",
					},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(deploy, hpa).
				Build()

			detector = detection.NewLabelDetector(fakeClient, ctrl.Log.WithName("test"))

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "default",
				DeploymentDetails: &sharedtypes.DeploymentDetails{
					Name: "web-app",
				},
			}

			labels := detector.DetectLabels(ctx, k8sCtx)
			Expect(labels.HPAEnabled).To(BeTrue())
		})
	})

	// ========================================
	// TC-DL-005: StatefulSet Detection
	// ========================================
	Describe("StatefulSet Detection", func() {
		It("TC-DL-005: should detect StatefulSet pod", func() {
			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "db",
					Namespace: "default",
				},
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "db-0",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind:       "StatefulSet",
							Name:       "db",
							Controller: boolPtr(true),
						},
					},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(sts, pod).
				Build()

			detector = detection.NewLabelDetector(fakeClient, ctrl.Log.WithName("test"))

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "default",
				PodDetails: &sharedtypes.PodDetails{
					Name: "db-0",
				},
			}

			labels := detector.DetectLabels(ctx, k8sCtx)
			Expect(labels.Stateful).To(BeTrue())
		})
	})

	// ========================================
	// TC-DL-006: Helm Detection
	// ========================================
	Describe("Helm Detection", func() {
		DescribeTable("should detect Helm management",
			func(labels map[string]string, annotations map[string]string, expectedHelm bool) {
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "web-app",
						Namespace:   "default",
						Labels:      labels,
						Annotations: annotations,
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithRuntimeObjects(deploy).
					Build()

				detector = detection.NewLabelDetector(fakeClient, ctrl.Log.WithName("test"))

				k8sCtx := &sharedtypes.KubernetesContext{
					Namespace: "default",
					DeploymentDetails: &sharedtypes.DeploymentDetails{
						Name:   "web-app",
						Labels: labels,
					},
				}

				result := detector.DetectLabels(ctx, k8sCtx)
				Expect(result.HelmManaged).To(Equal(expectedHelm))
			},
			Entry("TC-DL-006a: app.kubernetes.io/managed-by Helm label",
				map[string]string{"app.kubernetes.io/managed-by": "Helm"}, nil, true),
			Entry("TC-DL-006b: helm.sh/chart annotation",
				nil, map[string]string{"helm.sh/chart": "myapp-1.0.0"}, true),
			Entry("TC-DL-006c: Not Helm managed",
				nil, nil, false),
		)
	})

	// ========================================
	// TC-DL-007: NetworkPolicy Detection
	// ========================================
	Describe("NetworkPolicy Detection", func() {
		It("TC-DL-007: should detect NetworkPolicy in namespace", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "secure-ns",
				},
			}
			netpol := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deny-all",
					Namespace: "secure-ns",
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(ns, netpol).
				Build()

			detector = detection.NewLabelDetector(fakeClient, ctrl.Log.WithName("test"))

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "secure-ns",
			}

			labels := detector.DetectLabels(ctx, k8sCtx)
			Expect(labels.NetworkIsolated).To(BeTrue())
		})
	})

	// ========================================
	// TC-DL-008: ServiceMesh Detection
	// ========================================
	Describe("ServiceMesh Detection", func() {
		DescribeTable("should detect service mesh",
			func(annotations map[string]string, labels map[string]string, expectedMesh string) {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "web-pod",
						Namespace:   "default",
						Annotations: annotations,
						Labels:      labels,
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithRuntimeObjects(pod).
					Build()

				detector = detection.NewLabelDetector(fakeClient, ctrl.Log.WithName("test"))

				k8sCtx := &sharedtypes.KubernetesContext{
					Namespace: "default",
					PodDetails: &sharedtypes.PodDetails{
						Name:        "web-pod",
						Annotations: annotations,
						Labels:      labels,
					},
				}

				result := detector.DetectLabels(ctx, k8sCtx)
				Expect(result.ServiceMesh).To(Equal(expectedMesh))
			},
			Entry("TC-DL-008a: Istio sidecar injected",
				map[string]string{"sidecar.istio.io/status": `{"version":"1.17.2"}`}, nil, "istio"),
			Entry("TC-DL-008b: Linkerd proxy injected",
				map[string]string{"linkerd.io/proxy-version": "stable-2.13.0"}, nil, "linkerd"),
			Entry("TC-DL-008c: No service mesh",
				nil, nil, ""),
		)
	})

	// ========================================
	// FailedDetections Tracking (DD-WORKFLOW-001 v2.1)
	// ========================================
	Describe("Failed Detections Tracking", func() {
		It("should handle nil KubernetesContext gracefully", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			detector = detection.NewLabelDetector(fakeClient, ctrl.Log.WithName("test"))

			labels := detector.DetectLabels(ctx, nil)
			Expect(labels).To(BeNil())
		})

		It("should return empty FailedDetections when all queries succeed", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(ns).
				Build()

			detector = detection.NewLabelDetector(fakeClient, ctrl.Log.WithName("test"))

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "default",
			}

			labels := detector.DetectLabels(ctx, k8sCtx)
			Expect(labels.FailedDetections).To(BeEmpty())
		})
	})

	// ========================================
	// Edge Cases
	// ========================================
	Describe("Edge Cases", func() {
		It("should handle multiple labels detected simultaneously", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "multi-feature",
				},
			}
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "web-app",
					Namespace:   "multi-feature",
					Annotations: map[string]string{"argocd.argoproj.io/instance": "web-app"},
					Labels:      map[string]string{"app.kubernetes.io/managed-by": "Helm"},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "web"},
					},
				},
			}
			pdb := &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-pdb",
					Namespace: "multi-feature",
				},
				Spec: policyv1.PodDisruptionBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "web"},
					},
				},
			}
			hpa := &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-hpa",
					Namespace: "multi-feature",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind: "Deployment",
						Name: "web-app",
					},
				},
			}
			netpol := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deny-all",
					Namespace: "multi-feature",
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(ns, deploy, pdb, hpa, netpol).
				Build()

			detector = detection.NewLabelDetector(fakeClient, ctrl.Log.WithName("test"))

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "multi-feature",
				DeploymentDetails: &sharedtypes.DeploymentDetails{
					Name:   "web-app",
					Labels: map[string]string{"app": "web"},
				},
			}

			labels := detector.DetectLabels(ctx, k8sCtx)

			Expect(labels.GitOpsManaged).To(BeTrue())
			Expect(labels.GitOpsTool).To(Equal("argocd"))
			Expect(labels.HelmManaged).To(BeTrue())
			Expect(labels.PDBProtected).To(BeTrue())
			Expect(labels.HPAEnabled).To(BeTrue())
			Expect(labels.NetworkIsolated).To(BeTrue())
			Expect(labels.FailedDetections).To(BeEmpty())
		})
	})
})
