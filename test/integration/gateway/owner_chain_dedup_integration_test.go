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

// ========================================
// Owner Chain Deduplication Integration Test (#270)
// ========================================
//
// PURPOSE:
// Validates that the Gateway correctly deduplicates alerts targeting different
// pods from the same Deployment by resolving owner chains and generating
// Deployment-level fingerprints. This is the integration test for GitHub #270.
//
// ARCHITECTURE:
// - Creates real K8s objects (Deployment → ReplicaSet → Pods) in envtest
// - Wires K8sOwnerResolver into PrometheusAdapter (production-like pipeline)
// - Calls adapter.Parse() to trigger owner chain resolution
// - Calls ProcessSignal() to exercise deduplication logic
// - Asserts single RR created for two different pods from same Deployment
//
// BUSINESS REQUIREMENTS:
// - BR-GATEWAY-004: Owner-chain-based fingerprinting for deduplication
// ========================================

package gateway

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	gwTypes "github.com/jordigilh/kubernaut/pkg/gateway/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

var _ = Describe("Owner Chain Deduplication (#270, BR-GATEWAY-004)", Ordered, Label("owner-chain", "dedup", "integration"), func() {
	var (
		testLogger    logr.Logger
		testNamespace string
		gwServer      *gateway.Server
	)

	BeforeAll(func() {
		testLogger = logger.WithValues("test", "owner-chain-dedup-integration")

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Owner Chain Deduplication Integration Test (#270) - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		testNamespace = helpers.CreateTestNamespace(ctx, k8sClient, "ownerchain-int")
		testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)

		gwConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
		var err error
		gwServer, err = createGatewayServer(gwConfig, testLogger, k8sClient, sharedAuditStore)
		Expect(err).ToNot(HaveOccurred())
		testLogger.Info("✅ Gateway server initialized")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Owner Chain Deduplication - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if CurrentSpecReport().Failed() {
			testLogger.Info("⚠️  Test FAILED - Preserving namespace for debugging",
				"namespace", testNamespace)
			return
		}

		helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
		testLogger.Info("✅ Test cleanup complete")
	})

	Describe("Two pods from same Deployment produce one RR (#270 bug scenario)", func() {
		It("should deduplicate alerts from different pods of the same Deployment", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Two alerts for different pods → same Deployment fingerprint → 1 RR")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			By("1. Creating K8s owner chain: Deployment → ReplicaSet → 2 Pods")

			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app", Namespace: testNamespace,
					UID: k8stypes.UID("deploy-uid-270"),
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(2),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "leaky-app"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "leaky-app"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "main",
								Image: "busybox:latest",
							}},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, deploy)).To(Succeed())

			rs := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app-587f69c664", Namespace: testNamespace,
					UID: k8stypes.UID("rs-uid-270"),
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "Deployment", Name: "leaky-app",
						UID: k8stypes.UID("deploy-uid-270"), Controller: booleanPtr(true),
					}},
				},
				Spec: appsv1.ReplicaSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "leaky-app"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "leaky-app"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "main",
								Image: "busybox:latest",
							}},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, rs)).To(Succeed())

			podA := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app-587f69c664-abc12", Namespace: testNamespace,
					UID: k8stypes.UID("pod-a-uid"),
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "leaky-app-587f69c664",
						UID: k8stypes.UID("rs-uid-270"), Controller: booleanPtr(true),
					}},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "main",
						Image: "busybox:latest",
					}},
				},
			}
			Expect(k8sClient.Create(ctx, podA)).To(Succeed())

			podB := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app-587f69c664-xyz98", Namespace: testNamespace,
					UID: k8stypes.UID("pod-b-uid"),
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "leaky-app-587f69c664",
						UID: k8stypes.UID("rs-uid-270"), Controller: booleanPtr(true),
					}},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "main",
						Image: "busybox:latest",
					}},
				},
			}
			Expect(k8sClient.Create(ctx, podB)).To(Succeed())

			testLogger.Info("✅ Owner chain created",
				"deployment", "leaky-app",
				"replicaset", "leaky-app-587f69c664",
				"podA", "leaky-app-587f69c664-abc12",
				"podB", "leaky-app-587f69c664-xyz98")

			By("2. Creating OwnerResolver and PrometheusAdapter wired to envtest client")

			ownerResolver := adapters.NewK8sOwnerResolver(k8sClient)
			adapter := adapters.NewPrometheusAdapter(ownerResolver, nil)

			By("3. Parsing alert for Pod A (triggers owner chain resolution)")

			payloadA := createPrometheusAlertForPod(testNamespace, "ContainerMemoryExhaustionPredicted", "warning", "", "", "leaky-app-587f69c664-abc12")
			signalA, err := adapter.Parse(ctx, payloadA)
			Expect(err).ToNot(HaveOccurred())
			Expect(signalA.Fingerprint).ToNot(BeEmpty(), "Signal A should have a computed fingerprint")

			testLogger.Info("Signal A parsed",
				"fingerprint", signalA.Fingerprint,
				"resource.kind", signalA.Resource.Kind,
				"resource.name", signalA.Resource.Name)

			By("4. Parsing alert for Pod B (same Deployment, different pod)")

			payloadB := createPrometheusAlertForPod(testNamespace, "ContainerMemoryExhaustionPredicted", "warning", "", "", "leaky-app-587f69c664-xyz98")
			signalB, err := adapter.Parse(ctx, payloadB)
			Expect(err).ToNot(HaveOccurred())
			Expect(signalB.Fingerprint).ToNot(BeEmpty(), "Signal B should have a computed fingerprint")

			testLogger.Info("Signal B parsed",
				"fingerprint", signalB.Fingerprint,
				"resource.kind", signalB.Resource.Kind,
				"resource.name", signalB.Resource.Name)

			By("5. Asserting both signals produce the SAME Deployment-level fingerprint")

			Expect(signalA.Fingerprint).To(Equal(signalB.Fingerprint),
				"Alerts for different pods of the same Deployment must produce the same fingerprint")

			expectedFP := gwTypes.CalculateOwnerFingerprint(gwTypes.ResourceIdentifier{
				Namespace: testNamespace, Kind: "Deployment", Name: "leaky-app",
			})
			Expect(signalA.Fingerprint).To(Equal(expectedFP),
				"Fingerprint should be at Deployment level, not Pod level")

			testLogger.Info("✅ Fingerprints match at Deployment level",
				"fingerprint", signalA.Fingerprint,
				"expected", expectedFP)

			By("6. Processing both signals through Gateway → should create exactly 1 RR")

			responseA, err := gwServer.ProcessSignal(ctx, signalA)
			Expect(err).ToNot(HaveOccurred())
			Expect(responseA.Status).To(Equal("created"),
				"First signal should create a new RemediationRequest")

			testLogger.Info("Signal A processed",
				"status", responseA.Status,
				"rrName", responseA.RemediationRequestName)

			responseB, err := gwServer.ProcessSignal(ctx, signalB)
			Expect(err).ToNot(HaveOccurred())
			Expect(responseB.Duplicate).To(BeTrue(),
				"Second signal (same fingerprint) must be deduplicated")

			testLogger.Info("Signal B processed",
				"status", responseB.Status,
				"duplicate", responseB.Duplicate)

			By("7. Verifying exactly 1 RemediationRequest exists in controller namespace")

			rrList := listRRsByFingerprint(ctx, k8sClient, expectedFP)
			Expect(rrList).To(HaveLen(1),
				"Exactly one RR should exist for the Deployment fingerprint")

			testLogger.Info("✅ Owner chain deduplication validated: 2 pod alerts → 1 RR",
				"rrName", rrList[0].Name,
				"fingerprint", rrList[0].Spec.SignalFingerprint)
		})
	})

	Describe("Pod without owner chain uses pod-level fingerprint", func() {
		It("should use resource-level fingerprint for standalone pods", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Alert for standalone pod → pod-level fingerprint")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			By("1. Creating a standalone pod (no ownerReferences)")

			standalonePod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "standalone-worker", Namespace: testNamespace,
					UID: k8stypes.UID("standalone-uid"),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "worker",
						Image: "busybox:latest",
					}},
				},
			}
			Expect(k8sClient.Create(ctx, standalonePod)).To(Succeed())

			By("2. Parsing alert with OwnerResolver (should resolve to Pod itself)")

			ownerResolver := adapters.NewK8sOwnerResolver(k8sClient)
			adapter := adapters.NewPrometheusAdapter(ownerResolver, nil)

			payload := createPrometheusAlertForPod(testNamespace, "HighCPU", "warning", "", "", "standalone-worker")
			signal, err := adapter.Parse(ctx, payload)
			Expect(err).ToNot(HaveOccurred())

			expectedFP := gwTypes.CalculateOwnerFingerprint(gwTypes.ResourceIdentifier{
				Namespace: testNamespace, Kind: "Pod", Name: "standalone-worker",
			})
			Expect(signal.Fingerprint).To(Equal(expectedFP),
				"Standalone pod should use Pod-level fingerprint (no owner chain)")

			testLogger.Info("✅ Standalone pod uses pod-level fingerprint",
				"fingerprint", signal.Fingerprint)
		})
	})
})

func int32Ptr(i int32) *int32 { return &i }
func booleanPtr(b bool) *bool { return &b }

// listRRsByFingerprint lists RemediationRequests matching a given fingerprint
// across the controller namespace (kubernaut-system per ADR-057).
func listRRsByFingerprint(ctx context.Context, k8sClient client.Client, fingerprint string) []remediationv1alpha1.RemediationRequest {
	list := &remediationv1alpha1.RemediationRequestList{}
	err := k8sClient.List(ctx, list, client.InNamespace(controllerNamespace))
	Expect(err).ToNot(HaveOccurred())

	var matched []remediationv1alpha1.RemediationRequest
	for _, rr := range list.Items {
		if rr.Spec.SignalFingerprint == fingerprint {
			matched = append(matched, rr)
		}
	}
	return matched
}
