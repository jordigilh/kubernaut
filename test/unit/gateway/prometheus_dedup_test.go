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
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// newPrometheusAlertJSON creates a Prometheus AlertManager webhook JSON payload for testing.
// alertname, namespace, and resource labels are configurable.
func newPrometheusAlertJSON(alertname, namespace string, labels map[string]string) []byte {
	// Build label string
	labelParts := fmt.Sprintf(`"alertname": "%s", "namespace": "%s"`, alertname, namespace)
	for k, v := range labels {
		labelParts += fmt.Sprintf(`, "%s": "%s"`, k, v)
	}
	return []byte(fmt.Sprintf(`{
		"alerts": [{
			"status": "firing",
			"labels": {%s},
			"annotations": {"summary": "test alert"},
			"startsAt": "2026-02-09T10:00:00Z"
		}]
	}`, labelParts))
}

var _ = Describe("BR-GATEWAY-004: Prometheus Deduplication - Owner Chain Resolution", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	// Core business requirement: LLM investigates resource state, not signal type.
	// Multiple alert names for the same resource are redundant work.
	Describe("Same resource with different alertnames (Issue #63)", func() {
		It("should produce the same fingerprint regardless of alertname", func() {
			// Business scenario: kube-prometheus-stack fires both KubePodCrashLooping
			// and KubePodNotReady for the same crashing pod simultaneously.
			// LLM investigates the resource state, not the signal type.
			adapter := adapters.NewPrometheusAdapter(nil, nil)

			alert1 := newPrometheusAlertJSON("KubePodCrashLooping", "prod",
				map[string]string{"pod": "payment-api-789", "severity": "critical"})
			signal1, err := adapter.Parse(ctx, alert1)
			Expect(err).ToNot(HaveOccurred())

			alert2 := newPrometheusAlertJSON("KubePodNotReady", "prod",
				map[string]string{"pod": "payment-api-789", "severity": "warning"})
			signal2, err := adapter.Parse(ctx, alert2)
			Expect(err).ToNot(HaveOccurred())

			// BUSINESS OUTCOME: Both alerts target the same resource → same fingerprint
			// Prevents creating 2 RemediationRequests for the same root cause
			Expect(signal1.Fingerprint).To(Equal(signal2.Fingerprint),
				"Different alertnames for the same pod must produce the same fingerprint - LLM investigates resource state, not signal type")
		})

		It("should produce the same fingerprint for different alertnames targeting the same Deployment", func() {
			// Business scenario: Alerts targeting Deployment directly (no pod label)
			adapter := adapters.NewPrometheusAdapter(nil, nil)

			alert1 := newPrometheusAlertJSON("DeploymentReplicasMismatch", "prod",
				map[string]string{"deployment": "api-gateway", "severity": "warning"})
			signal1, err := adapter.Parse(ctx, alert1)
			Expect(err).ToNot(HaveOccurred())

			alert2 := newPrometheusAlertJSON("KubeDeploymentRolloutStuck", "prod",
				map[string]string{"deployment": "api-gateway", "severity": "critical"})
			signal2, err := adapter.Parse(ctx, alert2)
			Expect(err).ToNot(HaveOccurred())

			Expect(signal1.Fingerprint).To(Equal(signal2.Fingerprint),
				"Different alertnames for the same Deployment must produce the same fingerprint")
		})
	})

	// Owner-chain resolution: Pod → ReplicaSet → Deployment
	Describe("Owner chain resolution with OwnerResolver", func() {
		It("should resolve pod-level alerts to Deployment-level fingerprint", func() {
			// Mock: Pod "payment-api-789" is owned by Deployment "payment-api"
			resolver := &mockOwnerResolver{
				resolveFunc: func(ctx context.Context, namespace, kind, name string) (string, string, error) {
					if kind == "Pod" && name == "payment-api-789" {
						return "Deployment", "payment-api", nil
					}
					return kind, name, nil
				},
			}
			adapter := adapters.NewPrometheusAdapter(resolver, nil)

			alert := newPrometheusAlertJSON("KubePodCrashLooping", "prod",
				map[string]string{"pod": "payment-api-789", "severity": "critical"})
			signal, err := adapter.Parse(ctx, alert)
			Expect(err).ToNot(HaveOccurred())

			// Fingerprint should be based on the Deployment, not the Pod
			expectedFingerprint := types.CalculateOwnerFingerprint(types.ResourceIdentifier{
				Namespace: "prod",
				Kind:      "Deployment",
				Name:      "payment-api",
			})
			Expect(signal.Fingerprint).To(Equal(expectedFingerprint),
				"Pod-level alerts should be fingerprinted at the Deployment level via owner chain resolution")
		})

		It("should produce the same fingerprint for different pods owned by the same Deployment", func() {
			// Mock: Both pods are owned by Deployment "payment-api"
			resolver := &mockOwnerResolver{
				resolveFunc: func(ctx context.Context, namespace, kind, name string) (string, string, error) {
					return "Deployment", "payment-api", nil
				},
			}
			adapter := adapters.NewPrometheusAdapter(resolver, nil)

			alert1 := newPrometheusAlertJSON("KubePodCrashLooping", "prod",
				map[string]string{"pod": "payment-api-abc123", "severity": "critical"})
			signal1, err := adapter.Parse(ctx, alert1)
			Expect(err).ToNot(HaveOccurred())

			alert2 := newPrometheusAlertJSON("KubePodNotReady", "prod",
				map[string]string{"pod": "payment-api-def456", "severity": "warning"})
			signal2, err := adapter.Parse(ctx, alert2)
			Expect(err).ToNot(HaveOccurred())

			// BUSINESS OUTCOME: Different pods, different alertnames, same Deployment → same fingerprint
			Expect(signal1.Fingerprint).To(Equal(signal2.Fingerprint),
				"Different pods of the same Deployment with different alertnames must produce the same fingerprint")
		})
	})

	// Fallback: When OwnerResolver fails, fingerprint should still exclude alertname
	Describe("Fallback behavior on owner resolution failure", func() {
		It("should fall back to resource-level fingerprint without alertname when resolution fails", func() {
			resolver := &mockOwnerResolver{
				resolveFunc: func(ctx context.Context, namespace, kind, name string) (string, string, error) {
					return "", "", fmt.Errorf("RBAC: forbidden")
				},
			}
			adapter := adapters.NewPrometheusAdapter(resolver, nil)

			alert := newPrometheusAlertJSON("KubePodCrashLooping", "prod",
				map[string]string{"pod": "payment-api-789", "severity": "critical"})
			signal, err := adapter.Parse(ctx, alert)
			Expect(err).ToNot(HaveOccurred())

			// Fallback: resource-level fingerprint (no alertname, no owner chain)
			expectedFingerprint := types.CalculateOwnerFingerprint(types.ResourceIdentifier{
				Namespace: "prod",
				Kind:      "Pod",
				Name:      "payment-api-789",
			})
			Expect(signal.Fingerprint).To(Equal(expectedFingerprint),
				"On owner resolution failure, should fall back to resource-level fingerprint (alertname excluded)")
		})
	})

	// Without OwnerResolver: alertname is still excluded (architectural decision)
	Describe("Default behavior without OwnerResolver", func() {
		It("should exclude alertname from fingerprint even without OwnerResolver", func() {
			adapter := adapters.NewPrometheusAdapter(nil, nil)

			alert := newPrometheusAlertJSON("KubePodCrashLooping", "prod",
				map[string]string{"pod": "payment-api-789", "severity": "critical"})
			signal, err := adapter.Parse(ctx, alert)
			Expect(err).ToNot(HaveOccurred())

			// Without OwnerResolver, fingerprint is resource-level (no alertname)
			expectedFingerprint := types.CalculateOwnerFingerprint(types.ResourceIdentifier{
				Namespace: "prod",
				Kind:      "Pod",
				Name:      "payment-api-789",
			})
			Expect(signal.Fingerprint).To(Equal(expectedFingerprint),
				"Even without OwnerResolver, alertname must be excluded from fingerprint (LLM investigates resource state, not signal type)")
		})
	})
})
