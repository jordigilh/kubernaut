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
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newFullScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = autoscalingv2.AddToScheme(scheme)
	_ = policyv1.AddToScheme(scheme)
	_ = networkingv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	return scheme
}

var _ = Describe("Detected Labels Detection — TP-433-PARITY (#433)", func() {

	var (
		ctx      context.Context
		detector *enrichment.LabelDetector
	)

	Describe("UT-KA-433-DL-001: Detect GitOps management", func() {
		It("should detect ArgoCD GitOps management from root owner annotations", func() {
			scheme := newFullScheme()

			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-deploy",
					Namespace: "production",
					Annotations: map[string]string{
						"argocd.argoproj.io/managed-by": "argocd",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector = enrichment.NewLabelDetector(dynClient, newTestMapper())
			ctx = context.Background()

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "ReplicaSet", Name: "web-deploy-abc123", Namespace: "production"},
				{Kind: "Deployment", Name: "web-deploy", Namespace: "production"},
			}

			labels, err := detector.DetectLabels(ctx, "Pod", "web-deploy-pod-xyz", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue(), "should detect ArgoCD GitOps management")
			Expect(labels.GitOpsTool).To(Equal("argocd"), "should identify ArgoCD as the GitOps tool")
		})
	})

	Describe("UT-KA-433-DL-002: Detect HPA presence", func() {
		It("should detect HPA targeting the workload", func() {
			scheme := newFullScheme()

			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-deploy",
					Namespace: "production",
				},
			}
			hpa := &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-deploy-hpa",
					Namespace: "production",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "web-deploy",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy, hpa)
			detector = enrichment.NewLabelDetector(dynClient, newTestMapper())
			ctx = context.Background()

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "web-deploy", Namespace: "production"},
			}

			labels, err := detector.DetectLabels(ctx, "Deployment", "web-deploy", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.HPAEnabled).To(BeTrue(), "should detect HPA targeting the workload")
		})
	})

	Describe("UT-KA-433-DL-003: Detect PDB protection", func() {
		It("should detect PDB protecting the workload", func() {
			scheme := newFullScheme()

			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-deploy",
					Namespace: "production",
					Labels: map[string]string{
						"app": "web",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "web"},
					},
				},
			}
			minAvail := intstr.FromInt32(1)
			pdb := &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-pdb",
					Namespace: "production",
				},
				Spec: policyv1.PodDisruptionBudgetSpec{
					MinAvailable: &minAvail,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "web"},
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy, pdb)
			detector = enrichment.NewLabelDetector(dynClient, newTestMapper())
			ctx = context.Background()

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "web-deploy", Namespace: "production"},
			}

			labels, err := detector.DetectLabels(ctx, "Deployment", "web-deploy", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.PDBProtected).To(BeTrue(), "should detect PDB protection")
		})
	})

	Describe("UT-KA-433-DL-004: Detect Helm management", func() {
		It("should detect Helm management from labels", func() {
			scheme := newFullScheme()

			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-deploy",
					Namespace: "production",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "Helm",
						"helm.sh/chart":                "web-1.0.0",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector = enrichment.NewLabelDetector(dynClient, newTestMapper())
			ctx = context.Background()

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "web-deploy", Namespace: "production"},
			}

			labels, err := detector.DetectLabels(ctx, "Deployment", "web-deploy", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.HelmManaged).To(BeTrue(), "should detect Helm management")
		})
	})

	Describe("UT-KA-433-DL-005: Detect all 10 label fields from mixed resources", func() {
		It("should populate all applicable fields from a resource with multiple characteristics", func() {
			scheme := newFullScheme()

			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-deploy",
					Namespace: "production",
					Labels: map[string]string{
						"app":                          "web",
						"app.kubernetes.io/managed-by": "Helm",
					},
					Annotations: map[string]string{
						"argocd.argoproj.io/managed-by": "argocd",
						"sidecar.istio.io/inject":       "true",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "web"},
					},
				},
			}
			hpa := &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-hpa",
					Namespace: "production",
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "web-deploy",
					},
				},
			}
			pdb := &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-pdb",
					Namespace: "production",
				},
				Spec: policyv1.PodDisruptionBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "web"},
					},
					MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				},
			}
			netpol := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deny-all",
					Namespace: "production",
				},
			}
			quota := &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "production-quota",
					Namespace: "production",
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy, hpa, pdb, netpol, quota)
			detector = enrichment.NewLabelDetector(dynClient, newTestMapper())
			ctx = context.Background()

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "web-deploy", Namespace: "production"},
			}

			labels, err := detector.DetectLabels(ctx, "Deployment", "web-deploy", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue(), "GitOps should be detected")
			Expect(labels.GitOpsTool).To(Equal("argocd"))
			Expect(labels.HelmManaged).To(BeTrue(), "Helm should be detected")
			Expect(labels.HPAEnabled).To(BeTrue(), "HPA should be detected")
			Expect(labels.PDBProtected).To(BeTrue(), "PDB should be detected")
			Expect(labels.NetworkIsolated).To(BeTrue(), "NetworkPolicy should be detected")
			Expect(labels.ResourceQuotaConstrained).To(BeTrue(), "ResourceQuota should be detected")
			Expect(labels.ServiceMesh).To(Equal("istio"), "Istio should be detected")
			Expect(labels.Stateful).To(BeFalse(), "Deployment is not stateful")
			Expect(labels.FailedDetections).To(BeEmpty(), "no detections should fail")
		})
	})

	Describe("UT-KA-433-DL-006: Empty DetectedLabels when no characteristics found", func() {
		It("should return DetectedLabels with all false/empty when resource has no special characteristics", func() {
			scheme := newFullScheme()

			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "plain-deploy",
					Namespace: "default",
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector = enrichment.NewLabelDetector(dynClient, newTestMapper())
			ctx = context.Background()

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "plain-deploy", Namespace: "default"},
			}

			labels, err := detector.DetectLabels(ctx, "Deployment", "plain-deploy", "default", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeFalse())
			Expect(labels.HPAEnabled).To(BeFalse())
			Expect(labels.PDBProtected).To(BeFalse())
			Expect(labels.HelmManaged).To(BeFalse())
			Expect(labels.Stateful).To(BeFalse())
			Expect(labels.NetworkIsolated).To(BeFalse())
			Expect(labels.ResourceQuotaConstrained).To(BeFalse())
		})
	})
})
