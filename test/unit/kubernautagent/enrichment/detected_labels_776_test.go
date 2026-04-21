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

package enrichment_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

var _ = Describe("DD-HAPI-018 Parity — Issue #776", func() {

	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ═══════════════════════════════════════════════════════════════
	// GitOps Detection (DD-HAPI-018 Detection 1)
	// ═══════════════════════════════════════════════════════════════

	Describe("UT-KA-776-001: ArgoCD v3 tracking-id on root owner", func() {
		It("should detect gitOpsManaged=true from tracking-id annotation", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-server",
					Namespace: "production",
					Annotations: map[string]string{
						"argocd.argoproj.io/tracking-id": "my-app:apps/Deployment:production/api-server",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "api-server", Namespace: "production"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "api-server", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue(), "tracking-id annotation should trigger gitOpsManaged")
			Expect(labels.GitOpsTool).To(Equal("argocd"))
		})
	})

	Describe("UT-KA-776-002: ArgoCD v3 tracking-id on deployment annotations (pod template path)", func() {
		It("should detect gitOpsManaged=true from pod template tracking-id", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-app",
					Namespace: "staging",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"argocd.argoproj.io/tracking-id": "web-app:apps/Deployment:staging/web-app",
							},
						},
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "web-app", Namespace: "staging"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "web-app", "staging", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue(), "pod template tracking-id should trigger gitOpsManaged")
			Expect(labels.GitOpsTool).To(Equal("argocd"))
		})
	})

	Describe("UT-KA-776-003: ArgoCD v3 tracking-id on namespace annotations", func() {
		It("should detect gitOpsManaged=true from namespace tracking-id", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-app",
					Namespace: "argocd-managed-ns",
				},
			}
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "argocd-managed-ns",
					Annotations: map[string]string{
						"argocd.argoproj.io/tracking-id": "ns-app:core/Namespace:argocd-managed-ns",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy, ns)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "web-app", Namespace: "argocd-managed-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "web-app", "argocd-managed-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue(), "namespace-level tracking-id should trigger gitOpsManaged")
			Expect(labels.GitOpsTool).To(Equal("argocd"))
		})
	})

	Describe("UT-KA-776-004: tracking-id annotation + instance label coexist", func() {
		It("should detect gitOpsManaged=true with argocd (tracking-id wins by precedence)", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-app",
					Namespace: "production",
					Annotations: map[string]string{
						"argocd.argoproj.io/tracking-id": "my-app:apps/Deployment:production/web-app",
					},
					Labels: map[string]string{
						"argocd.argoproj.io/instance": "my-app",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "web-app", Namespace: "production"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "web-app", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue())
			Expect(labels.GitOpsTool).To(Equal("argocd"))
		})
	})

	Describe("UT-KA-776-005: DL-MX-01 Pod tracking-id + Deploy flux label -> argocd wins", func() {
		It("should select argocd (higher priority pod template tracking-id)", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mixed-app",
					Namespace: "production",
					Labels: map[string]string{
						"fluxcd.io/sync-gc-mark": "sha256:abc",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"argocd.argoproj.io/tracking-id": "mixed:apps/Deployment:production/mixed-app",
							},
						},
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "mixed-app", Namespace: "production"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "mixed-app", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue())
			Expect(labels.GitOpsTool).To(Equal("argocd"), "pod template tracking-id (priority 1) beats deploy flux label (priority 4)")
		})
	})

	Describe("UT-KA-776-006: DL-MX-02 Pod instance label + Deploy flux label -> argocd wins", func() {
		It("should select argocd (higher priority pod template instance label)", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mixed-app",
					Namespace: "production",
					Labels: map[string]string{
						"fluxcd.io/sync-gc-mark": "sha256:abc",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"argocd.argoproj.io/instance": "my-app",
							},
						},
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "mixed-app", Namespace: "production"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "mixed-app", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue())
			Expect(labels.GitOpsTool).To(Equal("argocd"), "pod template instance label (priority 2) beats deploy flux label (priority 4)")
		})
	})

	Describe("UT-KA-776-007: DL-MX-03 Deploy flux label + NS argocd label -> flux wins", func() {
		It("should select flux (deploy-level priority 4 beats NS-level priority 6)", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mixed-app",
					Namespace: "mixed-ns",
					Labels: map[string]string{
						"fluxcd.io/sync-gc-mark": "sha256:abc",
					},
				},
			}
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mixed-ns",
					Labels: map[string]string{
						"argocd.argoproj.io/instance": "ns-app",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy, ns)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "mixed-app", Namespace: "mixed-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "mixed-app", "mixed-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue())
			Expect(labels.GitOpsTool).To(Equal("flux"), "deploy flux label (priority 4) beats NS argocd label (priority 6)")
		})
	})

	Describe("UT-KA-776-008: DL-MX-04 Pod v3+v2 + Deploy v3+v2 -> argocd", func() {
		It("should select argocd from pod template tracking-id (highest priority)", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rich-app",
					Namespace: "production",
					Annotations: map[string]string{
						"argocd.argoproj.io/tracking-id": "deploy-level:apps/Deployment:production/rich-app",
					},
					Labels: map[string]string{
						"argocd.argoproj.io/instance": "deploy-label",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"argocd.argoproj.io/tracking-id": "pod-level:apps/Deployment:production/rich-app",
							},
							Labels: map[string]string{
								"argocd.argoproj.io/instance": "pod-label",
							},
						},
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "rich-app", Namespace: "production"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "rich-app", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue())
			Expect(labels.GitOpsTool).To(Equal("argocd"))
		})
	})

	Describe("UT-KA-776-009: Namespace annotation argocd.argoproj.io/managed -> argocd", func() {
		It("should detect gitOpsManaged from namespace managed annotation", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "plain-app",
					Namespace: "argocd-ns",
				},
			}
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "argocd-ns",
					Annotations: map[string]string{
						"argocd.argoproj.io/managed": "true",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy, ns)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "plain-app", Namespace: "argocd-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "plain-app", "argocd-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue(), "namespace managed annotation should trigger gitOpsManaged")
			Expect(labels.GitOpsTool).To(Equal("argocd"))
		})
	})

	Describe("UT-KA-776-010: Namespace annotation fluxcd.io/sync-status -> flux", func() {
		It("should detect gitOpsManaged from namespace flux sync-status annotation", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "plain-app",
					Namespace: "flux-ns",
				},
			}
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "flux-ns",
					Annotations: map[string]string{
						"fluxcd.io/sync-status": "synced",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy, ns)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "plain-app", Namespace: "flux-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "plain-app", "flux-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue(), "namespace sync-status annotation should trigger gitOpsManaged")
			Expect(labels.GitOpsTool).To(Equal("flux"))
		})
	})

	Describe("UT-KA-776-011: Cluster-scoped resource with tracking-id, NS check gracefully skipped", func() {
		It("should detect gitOps from tracking-id and skip NS check for cluster-scoped", func() {
			scheme := newFullScheme()
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
					Annotations: map[string]string{
						"argocd.argoproj.io/tracking-id": "cluster-app:core/Node:worker-1",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, node)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			labels, _, err := detector.DetectLabels(ctx, "Node", "worker-1", "", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue(), "cluster-scoped tracking-id should trigger gitOpsManaged")
			Expect(labels.GitOpsTool).To(Equal("argocd"))
		})
	})

	// ═══════════════════════════════════════════════════════════════
	// Service Mesh Detection (DD-HAPI-018 Detection 7)
	// ═══════════════════════════════════════════════════════════════

	Describe("UT-KA-776-012: Istio sidecar.istio.io/status on pod template", func() {
		It("should detect serviceMesh=istio from pod template status annotation", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "meshed-app",
					Namespace: "production",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"sidecar.istio.io/status": `{"initContainers":["istio-init"],"containers":["istio-proxy"]}`,
							},
						},
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "meshed-app", Namespace: "production"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "meshed-app", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.ServiceMesh).To(Equal("istio"), "pod template sidecar.istio.io/status should trigger serviceMesh=istio")
		})
	})

	Describe("UT-KA-776-013: Linkerd proxy-version on pod template", func() {
		It("should detect serviceMesh=linkerd from pod template proxy-version annotation", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "linkerd-app",
					Namespace: "production",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"linkerd.io/proxy-version": "stable-2.14.0",
							},
						},
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "linkerd-app", Namespace: "production"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "linkerd-app", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.ServiceMesh).To(Equal("linkerd"), "pod template linkerd.io/proxy-version should trigger serviceMesh=linkerd")
		})
	})

	Describe("UT-KA-776-014: Legacy Istio inject fallback on root owner", func() {
		It("should detect serviceMesh=istio from legacy root owner inject annotation", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "legacy-istio",
					Namespace: "production",
					Annotations: map[string]string{
						"sidecar.istio.io/inject": "true",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "legacy-istio", Namespace: "production"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "legacy-istio", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.ServiceMesh).To(Equal("istio"), "legacy inject annotation should still trigger serviceMesh=istio")
		})
	})

	Describe("UT-KA-776-015: Legacy Linkerd inject fallback on root owner", func() {
		It("should detect serviceMesh=linkerd from legacy root owner inject annotation", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "legacy-linkerd",
					Namespace: "production",
					Annotations: map[string]string{
						"linkerd.io/inject": "enabled",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "legacy-linkerd", Namespace: "production"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "legacy-linkerd", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.ServiceMesh).To(Equal("linkerd"), "legacy inject annotation should still trigger serviceMesh=linkerd")
		})
	})

	// ═══════════════════════════════════════════════════════════════
	// HPA Detection (DD-HAPI-018 Detection 3)
	// ═══════════════════════════════════════════════════════════════

	Describe("UT-KA-776-016: HPA targets root owner (existing behavior)", func() {
		It("should detect hpaEnabled=true when HPA targets root Deployment", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-server",
					Namespace: "production",
				},
			}
			hpa := &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-hpa",
					Namespace: "production",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind: "Deployment",
						Name: "api-server",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy, hpa)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "api-server", Namespace: "production"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "api-server", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.HPAEnabled).To(BeTrue(), "HPA targeting root Deployment should be detected")
		})
	})

	Describe("UT-KA-776-017: HPA targets intermediate owner in chain", func() {
		It("should detect hpaEnabled=true when HPA targets a ReplicaSet in the owner chain", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-server",
					Namespace: "production",
				},
			}
			hpa := &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-hpa",
					Namespace: "production",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind: "ReplicaSet",
						Name: "api-server-abc123",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy, hpa)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Pod", Name: "api-pod-xyz", Namespace: "production"},
				{Kind: "ReplicaSet", Name: "api-server-abc123", Namespace: "production"},
				{Kind: "Deployment", Name: "api-server", Namespace: "production"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Pod", "api-pod-xyz", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.HPAEnabled).To(BeTrue(), "HPA targeting intermediate ReplicaSet in chain should be detected")
		})
	})

	Describe("UT-KA-776-018: HPA targets StatefulSet root", func() {
		It("should detect hpaEnabled=true when HPA targets StatefulSet root", func() {
			scheme := newFullScheme()
			ss := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "db-cluster",
					Namespace: "production",
				},
			}
			hpa := &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "db-hpa",
					Namespace: "production",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind: "StatefulSet",
						Name: "db-cluster",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, ss, hpa)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "StatefulSet", Name: "db-cluster", Namespace: "production"},
			}

			labels, _, err := detector.DetectLabels(ctx, "StatefulSet", "db-cluster", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.HPAEnabled).To(BeTrue(), "HPA targeting StatefulSet root should be detected")
		})
	})

	// ═══════════════════════════════════════════════════════════════
	// Stateful Detection (DD-HAPI-018 Detection 4)
	// ═══════════════════════════════════════════════════════════════

	Describe("UT-KA-776-019: Owner chain [Pod, StatefulSet] -> stateful=true", func() {
		It("should detect stateful from root StatefulSet in chain", func() {
			scheme := newFullScheme()
			ss := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "db-cluster",
					Namespace: "production",
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, ss)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Pod", Name: "db-pod-0", Namespace: "production"},
				{Kind: "StatefulSet", Name: "db-cluster", Namespace: "production"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Pod", "db-pod-0", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.Stateful).To(BeTrue(), "StatefulSet in owner chain should trigger stateful=true")
		})
	})

	Describe("UT-KA-776-020: Owner chain [Pod, RS, StatefulSet] -> stateful=true", func() {
		It("should detect stateful by iterating the full owner chain", func() {
			scheme := newFullScheme()
			ss := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "db-cluster",
					Namespace: "production",
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, ss)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Pod", Name: "db-pod-0", Namespace: "production"},
				{Kind: "ReplicaSet", Name: "db-rs-abc", Namespace: "production"},
				{Kind: "StatefulSet", Name: "db-cluster", Namespace: "production"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Pod", "db-pod-0", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.Stateful).To(BeTrue(), "StatefulSet anywhere in owner chain should trigger stateful=true")
		})
	})

	// ═══════════════════════════════════════════════════════════════
	// ResourceQuota Detection (DD-HAPI-018 Detection 8)
	// ═══════════════════════════════════════════════════════════════

	Describe("UT-KA-776-021: Single ResourceQuota with hard/used summary", func() {
		It("should return resourceQuotaConstrained=true and populated quota summary", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-app",
					Namespace: "constrained-ns",
				},
			}
			quota := &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "compute-quota",
					Namespace: "constrained-ns",
				},
				Status: corev1.ResourceQuotaStatus{
					Hard: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("4"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
					Used: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy, quota)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "web-app", Namespace: "constrained-ns"},
			}

			labels, quotaDetails, err := detector.DetectLabels(ctx, "Deployment", "web-app", "constrained-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.ResourceQuotaConstrained).To(BeTrue())
			Expect(quotaDetails).NotTo(BeNil(), "quota summary should be populated")
			Expect(quotaDetails).To(HaveKey("cpu"))
			Expect(quotaDetails["cpu"].Hard).To(Equal("4"))
			Expect(quotaDetails["cpu"].Used).To(Equal("2"))
			Expect(quotaDetails).To(HaveKey("memory"))
			Expect(quotaDetails["memory"].Hard).To(Equal("8Gi"))
			Expect(quotaDetails["memory"].Used).To(Equal("4Gi"))
		})
	})

	Describe("UT-KA-776-022: Two ResourceQuotas with overlapping keys — first-wins", func() {
		It("should use first-wins semantics per resource key", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-app",
					Namespace: "multi-quota-ns",
				},
			}
			quota1 := &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "quota-alpha",
					Namespace: "multi-quota-ns",
				},
				Status: corev1.ResourceQuotaStatus{
					Hard: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("4"),
					},
					Used: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("2"),
					},
				},
			}
			quota2 := &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "quota-beta",
					Namespace: "multi-quota-ns",
				},
				Status: corev1.ResourceQuotaStatus{
					Hard: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("16Gi"),
					},
					Used: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("6"),
						corev1.ResourceMemory: resource.MustParse("12Gi"),
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy, quota1, quota2)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "web-app", Namespace: "multi-quota-ns"},
			}

			labels, quotaDetails, err := detector.DetectLabels(ctx, "Deployment", "web-app", "multi-quota-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.ResourceQuotaConstrained).To(BeTrue())
			Expect(quotaDetails).NotTo(BeNil())
			Expect(quotaDetails).To(HaveKey("memory"))
			Expect(quotaDetails["memory"].Hard).To(Equal("16Gi"))
		})
	})

	Describe("UT-KA-776-023: ResourceQuota with no status fields", func() {
		It("should return constrained=true with empty summary", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-app",
					Namespace: "empty-status-ns",
				},
			}
			quota := &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-quota",
					Namespace: "empty-status-ns",
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy, quota)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "web-app", Namespace: "empty-status-ns"},
			}

			labels, quotaDetails, err := detector.DetectLabels(ctx, "Deployment", "web-app", "empty-status-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.ResourceQuotaConstrained).To(BeTrue())
			Expect(quotaDetails).To(BeEmpty(), "empty status should yield empty quota summary")
		})
	})

	Describe("UT-KA-776-024: No ResourceQuotas -> nil summary", func() {
		It("should return constrained=false and nil summary", func() {
			scheme := newFullScheme()
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-app",
					Namespace: "no-quota-ns",
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "web-app", Namespace: "no-quota-ns"},
			}

			labels, quotaDetails, err := detector.DetectLabels(ctx, "Deployment", "web-app", "no-quota-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.ResourceQuotaConstrained).To(BeFalse())
			Expect(quotaDetails).To(BeNil(), "no RQs should yield nil summary")
		})
	})

	Describe("UT-KA-776-025: ResourceQuota API error -> failedDetections", func() {
		It("should mark resourceQuotaConstrained as failed and return nil summary on error", func() {
			scheme := newFullScheme()
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-node",
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, node)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			labels, quotaDetails, err := detector.DetectLabels(ctx, "Node", "worker-node", "", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.ResourceQuotaConstrained).To(BeFalse())
			Expect(labels.FailedDetections).To(ContainElement("resourceQuotaConstrained"))
			Expect(quotaDetails).To(BeNil(), "API error should yield nil summary")
		})
	})
})
