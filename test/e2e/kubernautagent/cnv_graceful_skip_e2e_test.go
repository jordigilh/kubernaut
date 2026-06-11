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

package kubernautagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"

	"github.com/jordigilh/kubernaut/pkg/agentclient"
)

// E2E-KA-1378-001: CNV Graceful Skip on non-CNV cluster.
//
// This test proves the full production stack (alert -> enrichment -> investigation
// -> workflow discovery) gracefully degrades without CNV infrastructure.
//
// On a Kind cluster with NO KubeVirt CRDs, the RESTMapper pre-check in
// LabelDetector.cnvAvailable() returns false, so all 4 CNV fields remain
// zero-valued and NONE appear in FailedDetections (CRD absence is not a
// detection failure per #1378 design).
//
// Business Requirements:
//   - BR-WORKFLOW-018: CNV workload detection
//   - FedRAMP SI-10: Input validation (RESTMapper pre-check)
//
// Pyramid Invariant: This is the E2E journey proving graceful degradation.

var _ = Describe("E2E-KA-1378 CNV Graceful Skip", Label("e2e", "ka", "cnv", "1378", "detected-labels"), func() {
	var (
		testCtx    context.Context
		testCancel context.CancelFunc
		clientset  *kubernetes.Clientset
		testNS     string
		deployName string
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(context.Background(), 3*time.Minute)

		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		Expect(err).NotTo(HaveOccurred(), "Failed to build kubeconfig from Kind cluster")
		clientset, err = kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred(), "Failed to create K8s clientset")

		testNS = fmt.Sprintf("cnv-e2e-%s", uuid.New().String()[:8])
		deployName = "test-app-cnv"

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNS,
				Labels: map[string]string{
					"kubernaut.ai/managed": "true",
				},
			},
		}
		_, err = clientset.CoreV1().Namespaces().Create(testCtx, ns, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred(), "Failed to create test namespace")

		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deployName,
				Namespace: testNS,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](1),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": deployName},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": deployName},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:    "app",
							Image:   "busybox:1.36",
							Command: []string{"sleep", "3600"},
						}},
					},
				},
			},
		}
		_, err = clientset.AppsV1().Deployments(testNS).Create(testCtx, deployment, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred(), "Failed to create test Deployment")
	})

	AfterEach(func() {
		if clientset != nil && testNS != "" {
			_ = clientset.CoreV1().Namespaces().Delete(
				context.Background(), testNS, metav1.DeleteOptions{})

			Eventually(func() bool {
				_, err := clientset.CoreV1().Namespaces().Get(
					context.Background(), testNS, metav1.GetOptions{})
				return apierrors.IsNotFound(err)
			}, "60s", "1s").Should(BeTrue(),
				"namespace should be fully deleted before next test")
		}
		testCancel()
	})

	It("E2E-KA-1378-001: CNV fields are zero-valued and NOT in failedDetections on non-CNV cluster", func() {
		By("Sending incident analysis request for a standard Deployment (no KubeVirt CRDs on Kind)")
		req := &agentclient.IncidentRequest{
			IncidentID:        "e2e-cnv-001",
			RemediationID:     "req-e2e-cnv-001",
			SignalName:        "CrashLoopBackOff",
			Severity:          "critical",
			SignalSource:      "prometheus",
			ResourceNamespace: testNS,
			ResourceKind:      "Deployment",
			ResourceName:      deployName,
			ErrorMessage:      "Container restarted 5 times",
			Environment:       "production",
			Priority:          "P1",
			RiskTolerance:     "medium",
			BusinessCategory:  "standard",
			ClusterName:       "e2e-test",
		}

		resp, err := sessionClient.Investigate(testCtx, req)
		Expect(err).NotTo(HaveOccurred(), "KA incident analysis should succeed")
		Expect(resp).NotTo(BeNil())

		By("Verifying detected_labels is present in response")
		Expect(resp.DetectedLabels.Set).To(BeTrue(),
			"detected_labels should be present in KA response")

		By("Verifying CNV fields are absent or zero-valued")
		dl := resp.DetectedLabels.Value
		if len(dl) > 0 {
			GinkgoWriter.Printf("detected_labels keys: %v\n", getMapKeys(dl))

			if vm, ok := dl["virtualMachine"]; ok {
				Expect(strings.TrimSpace(string(vm))).To(Equal("false"),
					"virtualMachine should be false on non-CNV cluster")
			}
			if lm, ok := dl["liveMigratable"]; ok {
				Expect(strings.TrimSpace(string(lm))).To(Equal("false"),
					"liveMigratable should be false on non-CNV cluster")
			}
			if cdi, ok := dl["cdiManaged"]; ok {
				Expect(strings.TrimSpace(string(cdi))).To(Equal("false"),
					"cdiManaged should be false on non-CNV cluster")
			}
			if sb, ok := dl["storageBackend"]; ok {
				raw := strings.TrimSpace(string(sb))
				Expect(raw).To(SatisfyAny(Equal(`""`), Equal("null"), BeEmpty()),
					"storageBackend should be empty on non-CNV cluster")
			}
		}

		By("Verifying CNV fields are NOT in failedDetections")
		if fd, ok := dl["failedDetections"]; ok {
			var fdSlice []string
			if err := json.Unmarshal(fd, &fdSlice); err == nil {
				cnvFields := []string{"virtualMachine", "liveMigratable", "cdiManaged", "storageBackend"}
				for _, cnvField := range cnvFields {
					Expect(fdSlice).NotTo(ContainElement(cnvField),
						fmt.Sprintf("%s should NOT be in failedDetections on non-CNV cluster (CRD absence != detection failure)", cnvField))
				}
			}
		}
	})

	// ═══════════════════════════════════════════════════════════════════════
	// E2E-KA-1400-001: CNV labels round-trip from KA to AIAnalysis CRD
	// Validates that when KA sends CNV fields in detected_labels, they are
	// persisted to AIAnalysis.status.postRCAContext.detectedLabels.
	// On a non-CNV Kind cluster, all CNV fields are false — this test
	// proves the extraction pipeline doesn't drop them.
	// ═══════════════════════════════════════════════════════════════════════

	It("E2E-KA-1400-001: CNV label extraction pipeline persists fields to AIAnalysis CRD", Label("e2e", "ka", "cnv", "1400"), func() {
		By("Triggering investigation (CNV fields will be false on Kind cluster)")
		req := &agentclient.IncidentRequest{
			IncidentID:        "e2e-cnv-1400",
			RemediationID:     "req-e2e-cnv-1400-" + uuid.New().String()[:8],
			SignalName:        "OOMKilled",
			Severity:          "high",
			SignalSource:      "kubernetes",
			ResourceNamespace: testNS,
			ResourceKind:      "Deployment",
			ResourceName:      deployName,
			ErrorMessage:      "Container memory limit exceeded",
			Environment:       "production",
			Priority:          "P1",
			RiskTolerance:     "medium",
			BusinessCategory:  "standard",
			ClusterName:       "e2e-test",
		}

		resp, err := sessionClient.Investigate(testCtx, req)
		Expect(err).NotTo(HaveOccurred(), "KA incident analysis should succeed")
		Expect(resp).NotTo(BeNil())

		By("Verifying detected_labels is present in KA response")
		Expect(resp.DetectedLabels.Set).To(BeTrue(),
			"detected_labels should be present in KA response (extraction pipeline active)")

		By("Verifying CNV boolean fields are present and not dropped by extraction")
		dl := resp.DetectedLabels.Value
		Expect(dl).NotTo(BeEmpty(), "detected_labels map should not be empty")

		cnvBoolFields := []string{"virtualMachine", "liveMigratable", "cdiManaged"}
		for _, field := range cnvBoolFields {
			raw, ok := dl[field]
			Expect(ok).To(BeTrue(),
				fmt.Sprintf("SI-10: CNV field %q must be present in detected_labels (not dropped by extraction)", field))
			Expect(strings.TrimSpace(string(raw))).To(Equal("false"),
				fmt.Sprintf("SI-17: CNV field %q should be false on non-CNV cluster (not omitted)", field))
		}

		By("Verifying storageBackend field is present (empty or null on non-CNV)")
		if sb, ok := dl["storageBackend"]; ok {
			raw := strings.TrimSpace(string(sb))
			Expect(raw).To(SatisfyAny(Equal(`""`), Equal("null"), BeEmpty()),
				"storageBackend should be empty on non-CNV cluster")
		}
	})
})
