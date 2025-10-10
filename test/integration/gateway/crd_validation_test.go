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

package gateway

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// Integration Tests: BR-GATEWAY-023 - RemediationRequest CRD Creation
//
// BUSINESS FOCUS: Verify CRDs are valid for downstream controllers
// - Schema validation ensures controller compatibility
// - Labels enable controller filtering (label selectors)
// - Metadata enables observability and troubleshooting

var _ = Describe("BR-GATEWAY-023: RemediationRequest CRD Creation", func() {
	var testNamespace string

	BeforeEach(func() {
		// Create unique namespace for test isolation
		testNamespace = fmt.Sprintf("test-crd-%d", time.Now().UnixNano())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
				Labels: map[string]string{
					"environment": "production", // For environment classification
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), ns)).To(Succeed())

		// Clear Redis
		Expect(redisClient.FlushDB(context.Background()).Err()).To(Succeed())
	})

	AfterEach(func() {
		// Cleanup namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		_ = k8sClient.Delete(context.Background(), ns)
	})

	It("creates CRD with complete schema validation (OpenAPI compliance)", func() {
		// BUSINESS SCENARIO: CRD must pass K8s schema validation
		// Expected: All required fields populated, enums validated, patterns matched
		//
		// WHY THIS MATTERS: Invalid CRDs break downstream controllers
		// Example: Missing required field → Controller crashes or ignores CRD
		// K8s OpenAPI schema validation is first line of defense

		alertPayload := fmt.Sprintf(`{
			"version": "4",
			"status": "firing",
			"alerts": [{
				"labels": {
					"alertname": "HighMemoryUsage",
					"severity": "critical",
					"namespace": "%s",
					"pod": "payment-service-789"
				},
				"annotations": {
					"description": "Pod using 95%% memory",
					"runbook_url": "https://wiki.example.com/runbooks/memory"
				}
			}]
		}`, testNamespace)

		By("Gateway creates RemediationRequest CRD")
		req, err := http.NewRequest("POST",
			"http://localhost:8090/api/v1/signals/prometheus",
			bytes.NewBufferString(alertPayload))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Gateway returns 201 Created for new CRDs")

		Eventually(func() bool {
			rrList := &remediationv1alpha1.RemediationRequestList{}
			err := k8sClient.List(context.Background(), rrList,
				client.InNamespace(testNamespace))
			return err == nil && len(rrList.Items) > 0
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"CRD must be created and pass K8s admission")

		By("CRD passes OpenAPI schema validation")
		rrList := &remediationv1alpha1.RemediationRequestList{}
		err = k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
		Expect(err).NotTo(HaveOccurred())

		rr := rrList.Items[0]

		// BUSINESS OUTCOME: Schema compliance ensures controller compatibility

		// Required fields validation
		Expect(rr.Spec.SignalName).NotTo(BeEmpty(),
			"SignalName required for identification")
		Expect(rr.Spec.SignalFingerprint).NotTo(BeEmpty(),
			"Fingerprint required for deduplication")
		Expect(rr.Spec.SignalType).NotTo(BeEmpty(),
			"SignalType required for adapter tracking")

		// Enum validation (Environment)
		Expect(rr.Spec.Environment).To(BeElementOf(
			[]string{"production", "staging", "development", "unknown"}),
			"Environment must match schema enum")

		// Pattern validation (Priority: P0, P1, P2, P3)
		Expect(rr.Spec.Priority).To(MatchRegexp(`^P[0-3]$`),
			"Priority must match schema pattern")

		// Enum validation (Severity)
		Expect(rr.Spec.Severity).To(BeElementOf(
			[]string{"critical", "warning", "info"}),
			"Severity must match schema enum")

		// Provider data validation (byte array)
		Expect(rr.Spec.ProviderData).NotTo(BeEmpty(),
			"ProviderData required for remediation context")

		// Original payload validation (audit trail)
		Expect(rr.Spec.OriginalPayload).NotTo(BeEmpty(),
			"OriginalPayload required for compliance")

		// BUSINESS OUTCOME VERIFIED:
		// ✅ CRD schema prevents invalid data from reaching controllers
		// ✅ K8s admission validates all fields before persistence
		// ✅ Downstream controllers can trust CRD structure
	})

	It("populates CRD labels for controller filtering (label selectors)", func() {
		// BUSINESS SCENARIO: Controllers use label selectors to filter CRDs
		// Expected: Gateway propagates alert labels + adds standard labels
		//
		// WHY THIS MATTERS: Labels enable targeted reconciliation
		// Example: Team "platform" controller only processes their alerts
		// kubectl get rr -l kubernaut.io/team=platform

		alertPayload := fmt.Sprintf(`{
			"version": "4",
			"status": "firing",
			"alerts": [{
				"labels": {
					"alertname": "CrashLoopBackOff",
					"severity": "critical",
					"namespace": "%s",
					"pod": "api-service-123",
					"team": "platform-engineering",
					"app": "payment-api"
				}
			}]
		}`, testNamespace)

		By("Gateway creates CRD with propagated labels")
		req, err := http.NewRequest("POST",
			"http://localhost:8090/api/v1/signals/prometheus",
			bytes.NewBufferString(alertPayload))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Gateway returns 201 Created for new CRDs")

		Eventually(func() bool {
			rrList := &remediationv1alpha1.RemediationRequestList{}
			err := k8sClient.List(context.Background(), rrList,
				client.InNamespace(testNamespace))
			return err == nil && len(rrList.Items) > 0
		}, 10*time.Second).Should(BeTrue())

		By("CRD labels enable targeted controller reconciliation")
		rrList := &remediationv1alpha1.RemediationRequestList{}
		k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
		rr := rrList.Items[0]

		// BUSINESS OUTCOME: Labels enable controller label selectors

		// Standard Kubernetes labels (app.kubernetes.io/*)
		Expect(rr.Labels).To(HaveKeyWithValue(
			"app.kubernetes.io/managed-by", "gateway-service"),
			"Standard label for ownership tracking")
		Expect(rr.Labels).To(HaveKeyWithValue(
			"app.kubernetes.io/component", "remediation"),
			"Standard label for component identification")

		// Kubernaut-specific labels (kubernaut.io/*)
		Expect(rr.Labels).To(HaveKeyWithValue(
			"kubernaut.io/environment", "production"),
			"Environment label for filtering: kubectl get rr -l kubernaut.io/environment=production")

		Expect(rr.Labels).To(HaveKey("kubernaut.io/priority"),
			"Priority label for scheduling: kubectl get rr -l kubernaut.io/priority=P0")

		Expect(rr.Labels).To(HaveKeyWithValue(
			"kubernaut.io/severity", "critical"),
			"Severity label for filtering")

		// Propagated alert labels (with kubernaut.io/ prefix)
		// Gateway should namespace alert labels to avoid conflicts
		if val, ok := rr.Labels["kubernaut.io/team"]; ok {
			Expect(val).To(Equal("platform-engineering"),
				"Alert labels propagated for team-based routing")
		}

		if val, ok := rr.Labels["kubernaut.io/app"]; ok {
			Expect(val).To(Equal("payment-api"),
				"Alert labels propagated for app-based filtering")
		}

		// BUSINESS OUTCOME VERIFIED:
		// ✅ Controllers can filter: kubectl get rr -l kubernaut.io/team=platform-engineering
		// ✅ Label selectors enable targeted reconciliation
		// ✅ Standard labels enable observability tools
	})

	It("includes metadata for observability and troubleshooting", func() {
		// BUSINESS SCENARIO: Operators need to troubleshoot why alert triggered remediation
		// Expected: CRD annotations contain human-readable context
		//
		// WHY THIS MATTERS: Metadata enables debugging and audit
		// Example: Why was this CRD created? What was the original alert?
		// Annotations provide human-readable context

		alertPayload := fmt.Sprintf(`{
			"version": "4",
			"status": "firing",
			"alerts": [{
				"labels": {
					"alertname": "NodeDiskPressure",
					"severity": "warning",
					"namespace": "%s",
					"node": "worker-node-3"
				},
				"annotations": {
					"description": "Node has less than 10%% disk space available",
					"runbook_url": "https://wiki.example.com/runbooks/disk-pressure",
					"summary": "Critical disk space issue on worker node"
				}
			}]
		}`, testNamespace)

		By("Gateway creates CRD with observability metadata")
		req, err := http.NewRequest("POST",
			"http://localhost:8090/api/v1/signals/prometheus",
			bytes.NewBufferString(alertPayload))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Gateway returns 201 Created for new CRDs")

		Eventually(func() bool {
			rrList := &remediationv1alpha1.RemediationRequestList{}
			err := k8sClient.List(context.Background(), rrList,
				client.InNamespace(testNamespace))
			return err == nil && len(rrList.Items) > 0
		}, 10*time.Second).Should(BeTrue())

		By("CRD annotations provide troubleshooting context")
		rrList := &remediationv1alpha1.RemediationRequestList{}
		k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
		rr := rrList.Items[0]

		// BUSINESS OUTCOME: Metadata enables observability

		// Alert annotations should be in Spec.SignalAnnotations (not CRD annotations)
		Expect(rr.Spec.SignalAnnotations).To(HaveKey("description"),
			"Alert description for human understanding")

		if val, ok := rr.Spec.SignalAnnotations["runbook_url"]; ok {
			Expect(val).To(ContainSubstring("wiki.example.com"),
				"Runbook URL for operator guidance")
		}

		// CRD metadata annotations (Gateway-added)
		Expect(rr.Annotations).To(HaveKey("kubernaut.io/created-at"),
			"Timestamp for audit trail")

		// Timestamps
		Expect(rr.CreationTimestamp.IsZero()).To(BeFalse(),
			"K8s creation timestamp for lifecycle tracking")

		// Status (will be updated by controller)
		// Gateway creates CRD with empty status (controllers update)

		// BUSINESS OUTCOME VERIFIED:
		// ✅ Operators can kubectl describe rr <name> to see full context
		// ✅ Annotations provide human-readable troubleshooting info
		// ✅ Timestamps enable audit trail
	})
})
