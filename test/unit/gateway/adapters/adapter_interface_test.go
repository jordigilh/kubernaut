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

package adapters

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// ============================================================================
// BUSINESS OUTCOME TESTS: Adapter Interface Compliance
// ============================================================================
//
// PURPOSE: Validate adapters provide correct business metadata for:
// - Dynamic HTTP route registration (BR-GATEWAY-001, BR-GATEWAY-002)
// - Observability and monitoring (signal source tracking)
// - API documentation and discoverability
//
// BUSINESS VALUE:
// - Operations team can identify signal sources in metrics/logs
// - Dynamic route registration enables adding adapters without code changes
// - API metadata supports self-documenting API endpoints
//
// NOT TESTING: Implementation details, internal data structures
// ============================================================================

var _ = Describe("Adapter Interface - Business Metadata", func() {
	Context("BR-GATEWAY-001: Prometheus Adapter Business Metadata", func() {
		var adapter adapters.RoutableAdapter

		BeforeEach(func() {
			adapter = adapters.NewPrometheusAdapter()
		})

		It("provides correct adapter name for metrics and logging", func() {
			// BUSINESS OUTCOME: Metrics labeled with "prometheus" source
			// Operations can filter: `gateway_signals_total{adapter="prometheus"}`
			name := adapter.Name()

			Expect(name).To(Equal("prometheus"),
				"Adapter name used in metrics labels and structured logging")
		})

		It("provides correct HTTP route for dynamic registration", func() {
			// BUSINESS OUTCOME: Gateway dynamically registers route
			// HTTP POST /api/v1/signals/prometheus → routed to this adapter
			route := adapter.GetRoute()

			Expect(route).To(Equal("/api/v1/signals/prometheus"),
				"Route must match AlertManager webhook configuration")
			Expect(route).To(HavePrefix("/api/v1/signals/"),
				"All signal endpoints must follow /api/v1/signals/{source} pattern")
		})

		It("provides adapter metadata for API observability", func() {
			// BUSINESS OUTCOME: API documentation shows adapter capabilities
			// Operations can verify: What content types? What headers required?
			metadata := adapter.GetMetadata()

			// Adapter identification
			Expect(metadata.Name).To(Equal("prometheus"),
				"Metadata name matches adapter name for consistency")
			Expect(metadata.Description).NotTo(BeEmpty(),
				"Description helps operations understand adapter purpose")

			// API contract validation
			Expect(metadata.SupportedContentTypes).To(ContainElement("application/json"),
				"Must accept AlertManager's JSON webhooks")

			// Security requirements (Prometheus doesn't require auth headers)
			Expect(metadata.RequiredHeaders).To(BeEmpty(),
				"Prometheus AlertManager doesn't require authentication headers")
		})
	})

	Context("BR-GATEWAY-002: Kubernetes Event Adapter Business Metadata", func() {
		var adapter adapters.RoutableAdapter

		BeforeEach(func() {
			adapter = adapters.NewKubernetesEventAdapter()
		})

		It("provides correct adapter name for metrics and logging", func() {
			// BUSINESS OUTCOME: Metrics labeled with "kubernetes-event" source
			// Operations can filter: `gateway_signals_total{adapter="kubernetes-event"}`
			name := adapter.Name()

			Expect(name).To(Equal("kubernetes-event"),
				"Adapter name used in metrics labels and structured logging")
		})

		It("provides correct HTTP route for dynamic registration", func() {
			// BUSINESS OUTCOME: Gateway dynamically registers route
			// HTTP POST /api/v1/signals/kubernetes-event → routed to this adapter
			route := adapter.GetRoute()

			Expect(route).To(Equal("/api/v1/signals/kubernetes-event"),
				"Route must match K8s Event webhook configuration")
			Expect(route).To(HavePrefix("/api/v1/signals/"),
				"All signal endpoints must follow /api/v1/signals/{source} pattern")
		})

		It("provides adapter metadata for API observability", func() {
			// BUSINESS OUTCOME: API documentation shows adapter capabilities
			// Operations can verify: What content types? What headers required?
			metadata := adapter.GetMetadata()

			// Adapter identification
			Expect(metadata.Name).To(Equal("kubernetes-event"),
				"Metadata name matches adapter name for consistency")
			Expect(metadata.Description).NotTo(BeEmpty(),
				"Description helps operations understand adapter purpose")

			// API contract validation
			Expect(metadata.SupportedContentTypes).To(ContainElement("application/json"),
				"Must accept K8s Event JSON payloads")

			// Security requirements (K8s Events require authentication)
			Expect(metadata.RequiredHeaders).To(ContainElement("Authorization"),
				"K8s Events must be authenticated (Bearer token required)")
		})
	})
})

var _ = Describe("Kubernetes Event Adapter - Signal Quality Validation", func() {
	var adapter *adapters.KubernetesEventAdapter

	BeforeEach(func() {
		adapter = adapters.NewKubernetesEventAdapter()
	})

	Context("BR-GATEWAY-003: Signal Validation for Business Quality", func() {
		It("accepts valid K8s Event signals for remediation", func() {
			// BUSINESS OUTCOME: Well-formed K8s Events are accepted
			// Remediation workflow can proceed for valid signals
			validSignal := &types.NormalizedSignal{
				AlertName:   "PodCrashLooping",
				Fingerprint: "k8s-event-fingerprint-abc123",
				Severity:    "critical",
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "payment-service-789",
				},
			}

			err := adapter.Validate(validSignal)

			Expect(err).NotTo(HaveOccurred(),
				"Valid signals must be accepted for remediation processing")
		})

		It("rejects signals missing alertName (business requirement)", func() {
			// BUSINESS OUTCOME: Cannot remediate without knowing WHAT failed
			// Gateway rejects early to prevent incomplete CRD creation
			invalidSignal := &types.NormalizedSignal{
				AlertName:   "", // MISSING
				Fingerprint: "fingerprint-123",
				Severity:    "critical",
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			}

			err := adapter.Validate(invalidSignal)

			Expect(err).To(HaveOccurred(),
				"Must reject signals without alertName - cannot identify issue")
			Expect(err.Error()).To(ContainSubstring("alertName"),
				"Error message must indicate which field is missing")
		})

		It("rejects signals missing fingerprint (deduplication requirement)", func() {
			// BUSINESS OUTCOME: Cannot deduplicate without fingerprint
			// Gateway rejects to prevent duplicate RemediationRequests
			invalidSignal := &types.NormalizedSignal{
				AlertName:   "PodCrashLooping",
				Fingerprint: "", // MISSING
				Severity:    "critical",
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			}

			err := adapter.Validate(invalidSignal)

			Expect(err).To(HaveOccurred(),
				"Must reject signals without fingerprint - deduplication will fail")
			Expect(err.Error()).To(ContainSubstring("fingerprint"),
				"Error message must indicate deduplication requirement")
		})

		It("rejects signals with empty severity (BR-GATEWAY-181 pass-through)", func() {
			// BUSINESS OUTCOME: Severity pass-through architecture
			// Gateway accepts ANY non-empty severity string (Sev1, P0, critical, HIGH, etc.)
			// SignalProcessing Rego policies determine normalized severity downstream
			// Authority: BR-GATEWAY-181, DD-SEVERITY-001 v1.1
			invalidSignal := &types.NormalizedSignal{
				AlertName:   "PodCrashLooping",
				Fingerprint: "fingerprint-123",
				Severity:    "", // INVALID (empty string)
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			}

			err := adapter.Validate(invalidSignal)

			Expect(err).To(HaveOccurred(),
				"BR-GATEWAY-181: Must reject empty severity")
			Expect(err.Error()).To(ContainSubstring("severity"),
				"Error message must indicate severity is required")
		})

		It("rejects signals missing resource kind (remediation target requirement)", func() {
			// BUSINESS OUTCOME: Cannot remediate without knowing WHAT to fix
			// Gateway rejects early - RO needs resource kind for workflow selection
			invalidSignal := &types.NormalizedSignal{
				AlertName:   "PodCrashLooping",
				Fingerprint: "fingerprint-123",
				Severity:    "critical",
				Resource: types.ResourceIdentifier{
					Kind: "", // MISSING
					Name: "test-pod",
				},
			}

			err := adapter.Validate(invalidSignal)

			Expect(err).To(HaveOccurred(),
				"Must reject signals without resource kind - cannot select workflow")
			Expect(err.Error()).To(ContainSubstring("kind"),
				"Error message must indicate missing resource kind")
		})

		It("rejects signals missing resource name (remediation target requirement)", func() {
			// BUSINESS OUTCOME: Cannot remediate without knowing WHICH instance to fix
			// Gateway rejects early - workflow execution needs specific resource name
			invalidSignal := &types.NormalizedSignal{
				AlertName:   "PodCrashLooping",
				Fingerprint: "fingerprint-123",
				Severity:    "critical",
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "", // MISSING
				},
			}

			err := adapter.Validate(invalidSignal)

			Expect(err).To(HaveOccurred(),
				"Must reject signals without resource name - cannot target remediation")
			Expect(err.Error()).To(ContainSubstring("name"),
				"Error message must indicate missing resource name")
		})

		It("accepts ANY non-empty severity string (BR-GATEWAY-181 pass-through)", func() {
			// BUSINESS OUTCOME: Severity pass-through architecture
			// Gateway accepts ANY severity scheme: standard, enterprise, PagerDuty, custom
			// - Standard: "critical", "warning", "info"
			// - Enterprise: "Sev1", "Sev2", "Sev3", "Sev4"
			// - PagerDuty: "P0", "P1", "P2", "P3"
			// - Custom: "HIGH", "MEDIUM", "LOW", "urgent", "normal"
			// SignalProcessing Rego policies normalize downstream
			// Authority: BR-GATEWAY-181, DD-SEVERITY-001 v1.1
			validSeverities := []string{
				"critical", "warning", "info", // Standard
				"Sev1", "Sev2", "Sev3", "Sev4", // Enterprise
				"P0", "P1", "P2", "P3", // PagerDuty
				"HIGH", "MEDIUM", "LOW", // Custom uppercase
				"urgent", "normal", // Custom lowercase
			}

			for _, severity := range validSeverities {
				signal := &types.NormalizedSignal{
					AlertName:   "TestAlert",
					Fingerprint: "fingerprint-123",
					Severity:    severity,
					Resource: types.ResourceIdentifier{
						Kind: "Pod",
						Name: "test-pod",
					},
				}

				err := adapter.Validate(signal)

				Expect(err).NotTo(HaveOccurred(),
					"BR-GATEWAY-181: Gateway must accept '%s' severity (pass-through)", severity)
			}
		})

		// GW-UNIT-ADP-015: BR-GATEWAY-005 Adapter Error Resilience
		Context("BR-GATEWAY-005: Adapter Error Non-Fatal", func() {
			It("[GW-UNIT-ADP-015] should handle adapter errors without crashing service", func() {
				// BR-GATEWAY-005: Adapter errors must not terminate Gateway
				// BUSINESS LOGIC: One bad payload should not affect other signals
				// Unit Test: Error handling without infrastructure

				adapter := adapters.NewPrometheusAdapter()

				// Malformed JSON payload
				malformedPayload := []byte(`{"alerts": [{"labels": {incomplete`)

				signal, err := adapter.Parse(nil, malformedPayload)

				// BUSINESS RULE: Parsing error should be returned (not panic)
				Expect(err).To(HaveOccurred(),
					"BR-GATEWAY-005: Malformed payload should return error")
				Expect(signal).To(BeNil(),
					"Invalid payload should not produce signal")

				// BUSINESS RULE: Adapter should remain functional after error
				validPayload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "Test",
						"namespace": "prod"
					}
				}]
			}`)

				signal2, err2 := adapter.Parse(nil, validPayload)
				Expect(err2).ToNot(HaveOccurred(),
					"BR-GATEWAY-005: Adapter should process valid signals after error")
				Expect(signal2).ToNot(BeNil())
			})

			It("[GW-UNIT-ADP-015] should provide actionable error messages", func() {
				// BR-GATEWAY-005: Error messages must help operators debug
				// BUSINESS LOGIC: Clear errors enable faster incident resolution
				// Unit Test: Error message quality

				adapter := adapters.NewPrometheusAdapter()

				// Empty payload
				emptyPayload := []byte(`{}`)

				_, err := adapter.Parse(nil, emptyPayload)

				// BUSINESS RULE: Error should indicate what's wrong
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("alert"),
					"BR-GATEWAY-005: Error should indicate missing alerts")
			})

			It("[GW-UNIT-ADP-015] should handle missing required fields gracefully", func() {
				// BR-GATEWAY-005: Missing fields should not cause panics
				// BUSINESS LOGIC: Defensive programming for external inputs
				// Unit Test: Edge case handling

				adapter := adapters.NewPrometheusAdapter()

				// Missing alertname
				payload := []byte(`{
				"alerts": [{
					"labels": {
						"namespace": "prod"
					}
				}]
			}`)

				signal, err := adapter.Parse(nil, payload)

				// BUSINESS RULE: Validation should catch missing required fields
				if signal != nil {
					validationErr := adapter.Validate(signal)
					Expect(validationErr).To(HaveOccurred(),
						"BR-GATEWAY-005: Missing alertname should fail validation")
				}

				// Either parsing or validation should catch the error
				hasError := (err != nil) || (signal != nil && adapter.Validate(signal) != nil)
				Expect(hasError).To(BeTrue(),
					"BR-GATEWAY-005: Missing required fields must be detected")
			})
		})
	})
})
