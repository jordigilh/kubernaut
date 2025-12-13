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

package adapters_test

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

		It("rejects signals with invalid severity (business classification)", func() {
			// BUSINESS OUTCOME: Severity determines remediation priority
			// Gateway rejects invalid severity to ensure correct prioritization
			invalidSignal := &types.NormalizedSignal{
				AlertName:   "PodCrashLooping",
				Fingerprint: "fingerprint-123",
				Severity:    "high", // INVALID (must be critical/warning/info)
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			}

			err := adapter.Validate(invalidSignal)

			Expect(err).To(HaveOccurred(),
				"Must reject signals with invalid severity")
			Expect(err.Error()).To(ContainSubstring("severity"),
				"Error message must indicate severity validation failure")
			Expect(err.Error()).To(ContainSubstring("critical/warning/info"),
				"Error message must show valid severity options")
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

		It("accepts all valid severities (critical, warning, info)", func() {
			// BUSINESS OUTCOME: All severity levels supported for different urgency
			// Critical → immediate remediation
			// Warning → scheduled remediation  
			// Info → monitoring only
			validSeverities := []string{"critical", "warning", "info"}

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
					"Severity '%s' must be accepted for business prioritization", severity)
			}
		})
	})
})

