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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
)

// Business Outcome Testing: Test WHAT the system does, not HOW it does it
//
// ❌ WRONG: "should extract namespace from labels" (tests implementation)
// ✅ RIGHT: "identifies which Kubernetes resource triggered the alert" (tests business outcome)

var _ = Describe("BR-GATEWAY-002: Signal Ingestion - Prometheus Adapter", func() {
	var (
		adapter *adapters.PrometheusAdapter
		ctx     context.Context
	)

	BeforeEach(func() {
		adapter = adapters.NewPrometheusAdapter()
		ctx = context.Background()
	})

	// BUSINESS OUTCOME: Gateway needs to identify which Kubernetes resource needs remediation
	// This enables the system to create RemediationRequest CRDs targeting the right resource
	Context("when receiving alerts about Kubernetes resources", func() {
		It("identifies which Pod triggered the alert for remediation targeting", func() {
			// Business scenario: AlertManager sends webhook for pod memory issue
			alertManagerWebhook := []byte(`{
				"version": "4",
				"status": "firing",
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "PodMemoryHigh",
						"namespace": "production",
						"pod": "payment-api-7d9f8c",
						"severity": "critical"
					},
					"annotations": {
						"summary": "Pod memory usage critical"
					},
					"startsAt": "2025-10-09T10:00:00Z"
				}]
			}`)

			signal, err := adapter.Parse(ctx, alertManagerWebhook)

			// BUSINESS OUTCOME: Gateway can identify the resource for remediation
			Expect(err).NotTo(HaveOccurred(),
				"Gateway must be able to process AlertManager webhooks")
			Expect(signal.Resource.Name).NotTo(BeEmpty(),
				"Must identify WHICH resource needs remediation")
			Expect(signal.Resource.Kind).To(Equal("Pod"),
				"Must identify WHAT TYPE of resource (Pod/Deployment/Node)")
			Expect(signal.Resource.Namespace).NotTo(BeEmpty(),
				"Must know WHERE the resource is (needed for kubectl commands)")

			// Business capability verified: Gateway knows "Pod payment-api-7d9f8c in production needs remediation"
		})

		It("identifies which Deployment triggered the alert", func() {
			webhook := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "DeploymentReplicasMismatch",
						"namespace": "staging",
						"deployment": "api-gateway",
						"severity": "warning"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, webhook)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Name).To(Equal("api-gateway"),
				"Must identify the Deployment needing remediation")
			Expect(signal.Resource.Kind).To(Equal("Deployment"),
				"Must distinguish Deployments from Pods")
		})

		It("identifies which Node triggered the alert", func() {
			webhook := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "NodeDiskPressure",
						"node": "worker-node-3",
						"severity": "critical"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, webhook)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Name).To(Equal("worker-node-3"),
				"Must identify which Node needs remediation")
			Expect(signal.Resource.Kind).To(Equal("Node"),
				"Must handle cluster-scoped resources")
		})
	})

	// BUSINESS OUTCOME: Gateway needs to preserve audit trail for compliance/debugging
	Context("when processing alerts for audit compliance", func() {
		It("preserves original webhook data for audit trail", func() {
			// Business need: Regulatory compliance requires audit trail of all alert triggers
			originalWebhook := []byte(`{"alerts":[{"labels":{"alertname":"Test","namespace":"prod"}}]}`)

			signal, err := adapter.Parse(ctx, originalWebhook)

			Expect(err).NotTo(HaveOccurred())
			Expect([]byte(signal.RawPayload)).To(Equal(originalWebhook),
				"Must preserve original webhook for audit/compliance requirements")
		})
	})
})

var _ = Describe("BR-GATEWAY-006: Signal Normalization Across Sources", func() {
	Context("when downstream services need to process signals uniformly", func() {
		It("normalizes signals so downstream doesn't need to know the source", func() {
			// Business outcome: Downstream RemediationRequest controller doesn't care
			// if signal came from Prometheus, Grafana, or Kubernetes Events

			prometheusAdapter := adapters.NewPrometheusAdapter()
			ctx := context.Background()

			prometheusWebhook := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "HighCPU",
						"namespace": "prod",
						"pod": "api-1"
					}
				}]
			}`)

			signal, err := prometheusAdapter.Parse(ctx, prometheusWebhook)

			Expect(err).NotTo(HaveOccurred())

			// BUSINESS OUTCOME: All signals have consistent structure
			Expect(signal.SignalName).NotTo(BeEmpty(),
				"All signals must have alert identification")
			Expect(signal.Resource.Kind).NotTo(BeEmpty(),
				"All signals must identify resource type")
			Expect(signal.Fingerprint).NotTo(BeEmpty(),
				"All signals must have fingerprint for deduplication")
			Expect(signal.Source).NotTo(BeEmpty(),
				"Must track source for observability, but downstream doesn't use it")

			// Business capability: Downstream services work with normalized format,
			// don't need adapter-specific logic
		})
	})
})

var _ = Describe("BR-GATEWAY-004: Signal Fingerprinting", func() {
	var (
		adapter *adapters.PrometheusAdapter
		ctx     context.Context
	)

	BeforeEach(func() {
		adapter = adapters.NewPrometheusAdapter()
		ctx = context.Background()
	})

	// BUSINESS OUTCOME: Prevent creating 20 RemediationRequest CRDs for the same alert firing every 30s
	Context("when same alert fires repeatedly", func() {
		It("generates identical fingerprints for duplicate alerts to prevent redundant remediation", func() {
			// Business scenario: AlertManager sends same alert every 30 seconds
			alert1 := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "PodCrashLoop",
						"namespace": "prod",
						"pod": "payment-api-1"
					},
					"startsAt": "2025-10-09T10:00:00Z"
				}]
			}`)

			alert2 := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "PodCrashLoop",
						"namespace": "prod",
						"pod": "payment-api-1"
					},
					"startsAt": "2025-10-09T10:00:30Z"
				}]
			}`)

			signal1, err := adapter.Parse(ctx, alert1)
			Expect(err).NotTo(HaveOccurred())

			signal2, err := adapter.Parse(ctx, alert2)
			Expect(err).NotTo(HaveOccurred())

			// BUSINESS OUTCOME: Identical alerts produce same fingerprint
			Expect(signal1.Fingerprint).To(Equal(signal2.Fingerprint),
				"Duplicate alerts must have same fingerprint to prevent creating multiple RemediationRequest CRDs")

			// Business capability: 20 identical alerts within 5 minutes → 1 CRD created
		})

		It("generates different fingerprints for different alerts to ensure each resource gets remediated", func() {
			// Business scenario: 10 different pods are failing - need 10 separate remediations
			podAlert1 := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "PodCrashLoop",
						"namespace": "prod",
						"pod": "payment-api-1"
					}
				}]
			}`)

			podAlert2 := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "PodCrashLoop",
						"namespace": "prod",
						"pod": "payment-api-2"
					}
				}]
			}`)

			signal1, err := adapter.Parse(ctx, podAlert1)
			Expect(err).NotTo(HaveOccurred())

			signal2, err := adapter.Parse(ctx, podAlert2)
			Expect(err).NotTo(HaveOccurred())

			// BUSINESS OUTCOME: Different resources get different fingerprints
			Expect(signal1.Fingerprint).NotTo(Equal(signal2.Fingerprint),
				"Different resources must have different fingerprints so each gets remediated separately")

			// Business capability: 10 pods failing → 10 CRDs created (or storm aggregation kicks in)
		})
	})
})

var _ = Describe("BR-GATEWAY-003: Payload Validation", func() {
	var (
		adapter *adapters.PrometheusAdapter
		ctx     context.Context
	)

	BeforeEach(func() {
		adapter = adapters.NewPrometheusAdapter()
		ctx = context.Background()
	})

	// BUSINESS OUTCOME: Protect downstream services from malformed/malicious data
	// Using DescribeTable per testing-strategy.md guidance
	DescribeTable("rejects invalid webhooks to protect system integrity",
		func(scenario string, payload []byte) {
			signal, parseErr := adapter.Parse(ctx, payload)

			// Business outcome: Invalid data is rejected before reaching downstream services
			if parseErr != nil {
				Expect(parseErr).To(HaveOccurred(),
					"Invalid webhooks must be rejected at Gateway boundary: %s", scenario)
			} else {
				// Some validation happens in Validate() after Parse()
				validateErr := adapter.Validate(signal)
				Expect(validateErr).To(HaveOccurred(),
					"Invalid signals must be caught before creating CRDs: %s", scenario)
			}
		},
		Entry("malformed JSON prevents processing",
			"Protects from syntax errors",
			[]byte(`{"alerts": [{"invalid json`)),
		Entry("empty webhook is meaningless",
			"Prevents wasting resources on empty data",
			[]byte(``)),
		Entry("webhook without alerts array",
			"AlertManager format requires alerts array",
			[]byte(`{"version": "4"}`)),
		Entry("webhook with empty alerts",
			"No alerts means nothing to remediate",
			[]byte(`{"alerts": []}`)),
		Entry("alert without alertname",
			"Can't identify what problem occurred",
			[]byte(`{"alerts": [{"labels": {"namespace": "prod"}}]}`)),
	)
})
