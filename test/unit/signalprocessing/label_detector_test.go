/*
Copyright 2025 Jordi Gil.

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

package signalprocessing_test

import (
	"context"
	"fmt"

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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/detection"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ============================================================================
// DAY 8: DETECTEDLABELS AUTO-DETECTION TESTS
// ============================================================================
//
// Business Requirements:
//   - BR-SP-101: DetectedLabels Auto-Detection (8 cluster characteristics)
//   - BR-SP-103: FailedDetections Tracking (RBAC, timeout, network errors)
//
// Authoritative Reference: DD-WORKFLOW-001 v2.3
//
// Test Matrix: 16 tests
//   - Happy Path: 9 tests (DL-HP-01 to DL-HP-09)
//   - Edge Cases: 3 tests (DL-EC-01 to DL-EC-03)
//   - Error Handling: 4 tests (DL-ER-01 to DL-ER-04)
//
// ============================================================================

var _ = Describe("LabelDetector", func() {
	var (
		ctx        context.Context
		scheme     *runtime.Scheme
		fakeClient client.Client
		detector   *detection.LabelDetector
		logger     = zap.New(zap.UseDevMode(true))
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Setup scheme with required types
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
		Expect(policyv1.AddToScheme(scheme)).To(Succeed())
		Expect(autoscalingv2.AddToScheme(scheme)).To(Succeed())
		Expect(networkingv1.AddToScheme(scheme)).To(Succeed())
	})

	// ========================================================================
	// CONSTRUCTOR TESTS
	// ========================================================================

	Describe("NewLabelDetector", func() {
		It("should create detector with valid dependencies", func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			detector = detection.NewLabelDetector(fakeClient, logger)
			Expect(detector).NotTo(BeNil())
		})
	})

	// ========================================================================
	// HAPPY PATH TESTS (DL-HP-01 to DL-HP-09)
	// ========================================================================

	Describe("DetectLabels - Happy Path", func() {

		// DL-HP-01: ArgoCD-annotated Deployment
		It("DL-HP-01: should detect ArgoCD GitOps management (BR-SP-101)", func() {
			// Arrange: Deployment with ArgoCD annotation
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-deployment",
					Namespace: "prod",
					Annotations: map[string]string{
						"argocd.argoproj.io/instance": "my-app",
					},
				},
			}
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()
			detector = detection.NewLabelDetector(fakeClient, logger)

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "prod",
				DeploymentDetails: &sharedtypes.DeploymentDetails{
					Name: "api-deployment",
					Labels: map[string]string{
						"app": "api",
					},
				},
				PodDetails: &sharedtypes.PodDetails{
					Name: "api-pod-abc",
					Annotations: map[string]string{
						"argocd.argoproj.io/instance": "my-app",
					},
				},
			}
			ownerChain := []sharedtypes.OwnerChainEntry{}

			// Act
			labels := detector.DetectLabels(ctx, k8sCtx, ownerChain)

			// Assert
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue())
			Expect(labels.GitOpsTool).To(Equal("argocd"))
			Expect(labels.FailedDetections).To(BeEmpty())
		})

		// DL-HP-02: Flux-labeled Deployment
		It("DL-HP-02: should detect Flux GitOps management (BR-SP-101)", func() {
			// Arrange: Deployment with Flux label
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-deployment",
					Namespace: "prod",
					Labels: map[string]string{
						"fluxcd.io/sync-gc-mark": "sha256:abc123",
					},
				},
			}
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment).Build()
			detector = detection.NewLabelDetector(fakeClient, logger)

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "prod",
				DeploymentDetails: &sharedtypes.DeploymentDetails{
					Name: "api-deployment",
					Labels: map[string]string{
						"fluxcd.io/sync-gc-mark": "sha256:abc123",
					},
				},
			}
			ownerChain := []sharedtypes.OwnerChainEntry{}

			// Act
			labels := detector.DetectLabels(ctx, k8sCtx, ownerChain)

			// Assert
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue())
			Expect(labels.GitOpsTool).To(Equal("flux"))
		})

		// DL-HP-03: Deployment with PDB
		It("DL-HP-03: should detect PodDisruptionBudget protection (BR-SP-101)", func() {
			// Arrange: Pod with matching PDB
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-pod-abc",
					Namespace: "prod",
					Labels: map[string]string{
						"app": "api",
					},
				},
			}
			pdb := &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-pdb",
					Namespace: "prod",
				},
				Spec: policyv1.PodDisruptionBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "api",
						},
					},
					MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				},
			}
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod, pdb).Build()
			detector = detection.NewLabelDetector(fakeClient, logger)

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "prod",
				PodDetails: &sharedtypes.PodDetails{
					Name: "api-pod-abc",
					Labels: map[string]string{
						"app": "api",
					},
				},
			}
			ownerChain := []sharedtypes.OwnerChainEntry{}

			// Act
			labels := detector.DetectLabels(ctx, k8sCtx, ownerChain)

			// Assert
			Expect(labels).NotTo(BeNil())
			Expect(labels.PDBProtected).To(BeTrue())
		})

		// DL-HP-04: Deployment with HPA
		It("DL-HP-04: should detect HorizontalPodAutoscaler (BR-SP-101)", func() {
			// Arrange: Deployment with targeting HPA
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-deployment",
					Namespace: "prod",
				},
			}
			hpa := &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-hpa",
					Namespace: "prod",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "api-deployment",
					},
					MinReplicas: int32Ptr(2),
					MaxReplicas: 10,
				},
			}
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment, hpa).Build()
			detector = detection.NewLabelDetector(fakeClient, logger)

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "prod",
				DeploymentDetails: &sharedtypes.DeploymentDetails{
					Name: "api-deployment",
				},
			}
			ownerChain := []sharedtypes.OwnerChainEntry{}

			// Act
			labels := detector.DetectLabels(ctx, k8sCtx, ownerChain)

			// Assert
			Expect(labels).NotTo(BeNil())
			Expect(labels.HPAEnabled).To(BeTrue())
		})

		// DL-HP-05: Owner chain contains StatefulSet
		It("DL-HP-05: should detect StatefulSet from owner chain (BR-SP-101)", func() {
			// Arrange: Owner chain with StatefulSet
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			detector = detection.NewLabelDetector(fakeClient, logger)

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "prod",
				PodDetails: &sharedtypes.PodDetails{
					Name: "db-pod-0",
				},
			}
			// Owner chain from Day 7: Pod -> StatefulSet
			ownerChain := []sharedtypes.OwnerChainEntry{
				{Namespace: "prod", Kind: "StatefulSet", Name: "db"},
			}

			// Act
			labels := detector.DetectLabels(ctx, k8sCtx, ownerChain)

			// Assert
			Expect(labels).NotTo(BeNil())
			Expect(labels.Stateful).To(BeTrue())
		})

		// DL-HP-06: Helm-managed Deployment
		It("DL-HP-06: should detect Helm management (BR-SP-101)", func() {
			// Arrange: Deployment with Helm labels
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			detector = detection.NewLabelDetector(fakeClient, logger)

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "prod",
				DeploymentDetails: &sharedtypes.DeploymentDetails{
					Name: "api-deployment",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "Helm",
						"helm.sh/chart":                "api-1.0.0",
					},
				},
			}
			ownerChain := []sharedtypes.OwnerChainEntry{}

			// Act
			labels := detector.DetectLabels(ctx, k8sCtx, ownerChain)

			// Assert
			Expect(labels).NotTo(BeNil())
			Expect(labels.HelmManaged).To(BeTrue())
		})

		// DL-HP-07: Namespace with NetworkPolicy
		It("DL-HP-07: should detect NetworkPolicy isolation (BR-SP-101)", func() {
			// Arrange: Namespace with NetworkPolicy
			netpol := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deny-all",
					Namespace: "prod",
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
					PolicyTypes: []networkingv1.PolicyType{
						networkingv1.PolicyTypeIngress,
						networkingv1.PolicyTypeEgress,
					},
				},
			}
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(netpol).Build()
			detector = detection.NewLabelDetector(fakeClient, logger)

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "prod",
			}
			ownerChain := []sharedtypes.OwnerChainEntry{}

			// Act
			labels := detector.DetectLabels(ctx, k8sCtx, ownerChain)

			// Assert
			Expect(labels).NotTo(BeNil())
			Expect(labels.NetworkIsolated).To(BeTrue())
		})

		// DL-HP-08: Istio sidecar-injected Pod
		It("DL-HP-08: should detect Istio service mesh (BR-SP-101)", func() {
			// Arrange: Pod with Istio sidecar annotation
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			detector = detection.NewLabelDetector(fakeClient, logger)

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "prod",
				PodDetails: &sharedtypes.PodDetails{
					Name: "api-pod-abc",
					Annotations: map[string]string{
						"sidecar.istio.io/status": `{"version":"1.18.0"}`,
					},
				},
			}
			ownerChain := []sharedtypes.OwnerChainEntry{}

			// Act
			labels := detector.DetectLabels(ctx, k8sCtx, ownerChain)

			// Assert
			Expect(labels).NotTo(BeNil())
			Expect(labels.ServiceMesh).To(Equal("istio"))
		})

		// DL-HP-09: Linkerd proxy-injected Pod
		It("DL-HP-09: should detect Linkerd service mesh (BR-SP-101)", func() {
			// Arrange: Pod with Linkerd proxy annotation
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			detector = detection.NewLabelDetector(fakeClient, logger)

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "prod",
				PodDetails: &sharedtypes.PodDetails{
					Name: "api-pod-abc",
					Annotations: map[string]string{
						"linkerd.io/proxy-version": "stable-2.14.0",
					},
				},
			}
			ownerChain := []sharedtypes.OwnerChainEntry{}

			// Act
			labels := detector.DetectLabels(ctx, k8sCtx, ownerChain)

			// Assert
			Expect(labels).NotTo(BeNil())
			Expect(labels.ServiceMesh).To(Equal("linkerd"))
		})
	})

	// ========================================================================
	// EDGE CASE TESTS (DL-EC-01 to DL-EC-03)
	// ========================================================================

	Describe("DetectLabels - Edge Cases", func() {

		// DL-EC-01: Clean deployment (no detections trigger)
		It("DL-EC-01: should return all false for clean deployment (BR-SP-101)", func() {
			// Arrange: Plain deployment with no special features
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			detector = detection.NewLabelDetector(fakeClient, logger)

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "default",
				DeploymentDetails: &sharedtypes.DeploymentDetails{
					Name:   "simple-app",
					Labels: map[string]string{"app": "simple"},
				},
				PodDetails: &sharedtypes.PodDetails{
					Name:   "simple-app-pod",
					Labels: map[string]string{"app": "simple"},
				},
			}
			ownerChain := []sharedtypes.OwnerChainEntry{
				{Namespace: "default", Kind: "ReplicaSet", Name: "simple-app-rs"},
				{Namespace: "default", Kind: "Deployment", Name: "simple-app"},
			}

			// Act
			labels := detector.DetectLabels(ctx, k8sCtx, ownerChain)

			// Assert - All detections should be false
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeFalse())
			Expect(labels.GitOpsTool).To(BeEmpty())
			Expect(labels.PDBProtected).To(BeFalse())
			Expect(labels.HPAEnabled).To(BeFalse())
			Expect(labels.Stateful).To(BeFalse())
			Expect(labels.HelmManaged).To(BeFalse())
			Expect(labels.NetworkIsolated).To(BeFalse())
			Expect(labels.ServiceMesh).To(BeEmpty())
			// No failures - just no features detected
			Expect(labels.FailedDetections).To(BeEmpty())
		})

		// DL-EC-02: Nil KubernetesContext
		It("DL-EC-02: should return nil for nil KubernetesContext (BR-SP-101)", func() {
			// Arrange
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			detector = detection.NewLabelDetector(fakeClient, logger)

			// Act
			labels := detector.DetectLabels(ctx, nil, nil)

			// Assert
			Expect(labels).To(BeNil())
		})

		// DL-EC-03: Multiple detections true simultaneously
		It("DL-EC-03: should detect multiple features simultaneously (BR-SP-101)", func() {
			// Arrange: Deployment with GitOps + PDB + HPA
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-deployment",
					Namespace: "prod",
					Annotations: map[string]string{
						"argocd.argoproj.io/instance": "my-app",
					},
					Labels: map[string]string{
						"app": "api",
					},
				},
			}
			pdb := &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-pdb",
					Namespace: "prod",
				},
				Spec: policyv1.PodDisruptionBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "api"},
					},
				},
			}
			hpa := &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-hpa",
					Namespace: "prod",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind: "Deployment",
						Name: "api-deployment",
					},
					MaxReplicas: 10,
				},
			}
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(deployment, pdb, hpa).Build()
			detector = detection.NewLabelDetector(fakeClient, logger)

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "prod",
				DeploymentDetails: &sharedtypes.DeploymentDetails{
					Name:   "api-deployment",
					Labels: map[string]string{"app": "api"},
				},
				PodDetails: &sharedtypes.PodDetails{
					Name:   "api-pod",
					Labels: map[string]string{"app": "api"},
					Annotations: map[string]string{
						"argocd.argoproj.io/instance": "my-app",
					},
				},
			}
			ownerChain := []sharedtypes.OwnerChainEntry{}

			// Act
			labels := detector.DetectLabels(ctx, k8sCtx, ownerChain)

			// Assert - Multiple features detected
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue())
			Expect(labels.GitOpsTool).To(Equal("argocd"))
			Expect(labels.PDBProtected).To(BeTrue())
			Expect(labels.HPAEnabled).To(BeTrue())
			Expect(labels.FailedDetections).To(BeEmpty())
		})
	})

	// ========================================================================
	// ERROR HANDLING TESTS (DL-ER-01 to DL-ER-04)
	// ========================================================================

	Describe("DetectLabels - Error Handling", func() {

		// DL-ER-01: RBAC denied (PDB query)
		It("DL-ER-01: should track PDB query failure in FailedDetections (BR-SP-103)", func() {
			// Arrange: Simulate RBAC forbidden on PDB list
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
						if _, ok := list.(*policyv1.PodDisruptionBudgetList); ok {
							return fmt.Errorf("forbidden: User cannot list poddisruptionbudgets")
						}
						return c.List(ctx, list, opts...)
					},
				}).Build()
			detector = detection.NewLabelDetector(fakeClient, logger)

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "prod",
				PodDetails: &sharedtypes.PodDetails{
					Name:   "api-pod",
					Labels: map[string]string{"app": "api"},
				},
			}
			ownerChain := []sharedtypes.OwnerChainEntry{}

			// Act
			labels := detector.DetectLabels(ctx, k8sCtx, ownerChain)

			// Assert
			Expect(labels).NotTo(BeNil())
			Expect(labels.PDBProtected).To(BeFalse()) // Default to false on error
			Expect(labels.FailedDetections).To(ContainElement("pdbProtected"))
		})

		// DL-ER-02: API timeout (HPA query)
		It("DL-ER-02: should track HPA query failure in FailedDetections (BR-SP-103)", func() {
			// Arrange: Simulate timeout on HPA list
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
						if _, ok := list.(*autoscalingv2.HorizontalPodAutoscalerList); ok {
							return fmt.Errorf("context deadline exceeded")
						}
						return c.List(ctx, list, opts...)
					},
				}).Build()
			detector = detection.NewLabelDetector(fakeClient, logger)

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "prod",
				DeploymentDetails: &sharedtypes.DeploymentDetails{
					Name: "api-deployment",
				},
			}
			ownerChain := []sharedtypes.OwnerChainEntry{}

			// Act
			labels := detector.DetectLabels(ctx, k8sCtx, ownerChain)

			// Assert
			Expect(labels).NotTo(BeNil())
			Expect(labels.HPAEnabled).To(BeFalse()) // Default to false on error
			Expect(labels.FailedDetections).To(ContainElement("hpaEnabled"))
		})

		// DL-ER-03: Multiple query failures
		It("DL-ER-03: should track multiple failures in FailedDetections (BR-SP-103)", func() {
			// Arrange: Simulate failures on PDB, HPA, and NetworkPolicy queries
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
						switch list.(type) {
						case *policyv1.PodDisruptionBudgetList:
							return fmt.Errorf("RBAC: access denied")
						case *autoscalingv2.HorizontalPodAutoscalerList:
							return fmt.Errorf("context deadline exceeded")
						case *networkingv1.NetworkPolicyList:
							return fmt.Errorf("connection refused")
						}
						return c.List(ctx, list, opts...)
					},
				}).Build()
			detector = detection.NewLabelDetector(fakeClient, logger)

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "prod",
				DeploymentDetails: &sharedtypes.DeploymentDetails{
					Name: "api-deployment",
				},
				PodDetails: &sharedtypes.PodDetails{
					Name:   "api-pod",
					Labels: map[string]string{"app": "api"},
				},
			}
			ownerChain := []sharedtypes.OwnerChainEntry{}

			// Act
			labels := detector.DetectLabels(ctx, k8sCtx, ownerChain)

			// Assert - All three should be in FailedDetections
			Expect(labels).NotTo(BeNil())
			Expect(labels.PDBProtected).To(BeFalse())
			Expect(labels.HPAEnabled).To(BeFalse())
			Expect(labels.NetworkIsolated).To(BeFalse())
			Expect(labels.FailedDetections).To(ContainElements(
				"pdbProtected",
				"hpaEnabled",
				"networkIsolated",
			))
		})

		// DL-ER-04: Context cancellation
		It("DL-ER-04: should return partial results on context cancellation (BR-SP-103)", func() {
			// Arrange: Cancel context during HPA query
			cancelCtx, cancel := context.WithCancel(ctx)

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
						if _, ok := list.(*autoscalingv2.HorizontalPodAutoscalerList); ok {
							cancel() // Cancel context
							return ctx.Err()
						}
						return c.List(ctx, list, opts...)
					},
				}).Build()
			detector = detection.NewLabelDetector(fakeClient, logger)

			k8sCtx := &sharedtypes.KubernetesContext{
				Namespace: "prod",
				DeploymentDetails: &sharedtypes.DeploymentDetails{
					Name: "api-deployment",
				},
			}
			ownerChain := []sharedtypes.OwnerChainEntry{}

			// Act
			labels := detector.DetectLabels(cancelCtx, k8sCtx, ownerChain)

			// Assert - Should return partial results with failure tracked
			Expect(labels).NotTo(BeNil())
			// HPA detection should fail
			Expect(labels.FailedDetections).To(ContainElement("hpaEnabled"))
		})
	})
})

// Helper function to create int32 pointer
func int32Ptr(i int32) *int32 {
	return &i
}



