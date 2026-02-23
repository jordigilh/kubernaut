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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// ============================================================================
// BUSINESS OUTCOME TESTS: Adapter Registry for Multi-Source Signal Routing
// ============================================================================
//
// BR-GATEWAY-001: Gateway must route signals from multiple sources
//
// BUSINESS VALUE:
// - Operators can configure which alert sources are enabled
// - HTTP server creates correct routes for each adapter
// - Duplicate adapter registration is prevented (configuration safety)
// ============================================================================

var _ = Describe("BR-GATEWAY-001: Adapter Registry enables multi-source signal ingestion", func() {
	var registry *adapters.AdapterRegistry

	BeforeEach(func() {
		registry = adapters.NewAdapterRegistry(zap.New(zap.UseDevMode(true)))
	})

	Context("Gateway startup requires adapter registration", func() {
		It("validates no adapters registered before configuration", func() {
			// BUSINESS OUTCOME: Gateway can fail-fast if no adapters configured
			// Enables: if registry.Count() == 0 { log.Fatal("No signal sources") }
			Expect(registry.Count()).To(Equal(0),
				"Empty registry enables startup validation - prevents Gateway with no signal sources")
		})

		It("tracks registered adapter count for startup validation", func() {
			// BUSINESS OUTCOME: Operators know how many signal sources are active
			_ = registry.Register(adapters.NewPrometheusAdapter())
			_ = registry.Register(adapters.NewKubernetesEventAdapter())

			Expect(registry.Count()).To(Equal(2),
				"Two signal sources configured - Prometheus and K8s Events")
		})
	})

	Context("HTTP route registration for each signal source", func() {
		It("exposes Prometheus route for AlertManager webhook integration", func() {
			// BUSINESS OUTCOME: AlertManager can POST to /api/v1/signals/prometheus
			_ = registry.Register(adapters.NewPrometheusAdapter())

			adapter, found := registry.GetAdapter("prometheus")

			Expect(found).To(BeTrue(), "Prometheus adapter must be findable by name")
			Expect(adapter.GetRoute()).To(Equal("/api/v1/signals/prometheus"),
				"Route enables AlertManager integration: POST /api/v1/signals/prometheus")
		})

		It("exposes K8s Event route for cluster event forwarding", func() {
			// BUSINESS OUTCOME: K8s Event forwarder can POST to /api/v1/signals/kubernetes-event
			_ = registry.Register(adapters.NewKubernetesEventAdapter())

			adapter, found := registry.GetAdapter("kubernetes-event")

			Expect(found).To(BeTrue(), "K8s Event adapter must be findable by name")
			Expect(adapter.GetRoute()).To(Equal("/api/v1/signals/kubernetes-event"),
				"Route enables K8s Event forwarding integration")
		})
	})

	Context("Configuration safety prevents duplicate registration", func() {
		It("rejects duplicate adapter registration with clear error", func() {
			// BUSINESS OUTCOME: Misconfiguration detected at startup, not runtime
			// Prevents: accidental double-registration overwriting adapter settings
			_ = registry.Register(adapters.NewPrometheusAdapter())

			err := registry.Register(adapters.NewPrometheusAdapter())

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already registered"),
				"Clear error helps operator diagnose configuration issue")
		})
	})

	Context("Runtime adapter lookup for request handling", func() {
		BeforeEach(func() {
			_ = registry.Register(adapters.NewPrometheusAdapter())
		})

		It("retrieves correct adapter for incoming HTTP request", func() {
			// BUSINESS OUTCOME: HTTP handler routes request to correct adapter
			adapter, found := registry.GetAdapter("prometheus")

			Expect(found).To(BeTrue())
			Expect(adapter.Name()).To(Equal("prometheus"),
				"Correct adapter returned for request handling")
		})

		It("returns not-found for unknown signal sources", func() {
			// BUSINESS OUTCOME: Unknown source requests return 404 (not panic)
			_, found := registry.GetAdapter("unknown-source")

			Expect(found).To(BeFalse(),
				"Unknown source returns false - HTTP handler returns 404")
		})

		It("provides all adapters for route registration at startup", func() {
			// BUSINESS OUTCOME: HTTP server can register routes for all adapters
			// Usage: for _, a := range GetAllAdapters() { mux.HandleFunc(a.GetRoute(), ...) }
			allAdapters := registry.GetAllAdapters()

			Expect(len(allAdapters)).To(Equal(1),
				"All registered adapters returned for HTTP route setup")
		})
	})
})

// ============================================================================
// BUSINESS OUTCOME TESTS: Signal Validation Before CRD Creation
// ============================================================================
//
// BR-GATEWAY-003: Signals must be validated before creating RemediationRequest CRD
//
// BUSINESS VALUE:
// - Invalid signals rejected early (don't waste K8s API calls)
// - Clear error messages help debug webhook misconfiguration
// - Valid signals proceed to CRD creation
// ============================================================================

var _ = Describe("BR-GATEWAY-003: Prometheus signal validation prevents invalid CRD creation", func() {
	var adapter *adapters.PrometheusAdapter

	BeforeEach(func() {
		adapter = adapters.NewPrometheusAdapter()
	})

	Context("Fingerprint required for deduplication", func() {
		It("rejects signals without fingerprint - deduplication impossible", func() {
			// BUSINESS OUTCOME: Cannot deduplicate signals without fingerprint
			// Prevents: duplicate RemediationRequest CRDs for same alert
			signal := &types.NormalizedSignal{
				SignalName: "HighMemoryUsage",
				Severity:  "critical",
				// Missing: Fingerprint
			}

			err := adapter.Validate(signal)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("fingerprint"),
				"Clear error: fingerprint required for deduplication")
		})
	})

	Context("AlertName required for workflow selection", func() {
		It("rejects signals without alert name - workflow selection impossible", func() {
			// BUSINESS OUTCOME: RO cannot select workflow without alert name
			signal := &types.NormalizedSignal{
				Fingerprint: "abc123def456",
				Severity:    "warning",
				// Missing: AlertName
			}

			err := adapter.Validate(signal)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("alertName"),
				"Clear error: alertName required for workflow selection")
		})
	})

	Context("Severity pass-through for downstream policy (BR-GATEWAY-181)", func() {
		It("accepts ANY non-empty severity - pass-through architecture", func() {
			// BUSINESS OUTCOME: Gateway passes through external severity values
			// SignalProcessing Rego policies determine normalized severity downstream
			// Authority: BR-GATEWAY-181, DD-SEVERITY-001 v1.1
			testCases := []struct {
				severity string
				scheme   string
			}{
				{"critical", "standard"},
				{"warning", "standard"},
				{"info", "standard"},
				{"Sev1", "enterprise"},
				{"P0", "PagerDuty"},
				{"HIGH", "custom"},
			}

			for _, tc := range testCases {
				signal := &types.NormalizedSignal{
					Fingerprint: "abc123",
					SignalName:   "TestAlert",
					Severity:    tc.severity,
				}

				err := adapter.Validate(signal)

				Expect(err).NotTo(HaveOccurred(),
					"BR-GATEWAY-181: Must accept '%s' (%s scheme)", tc.severity, tc.scheme)
			}
		})

		It("rejects empty severity - required for downstream policy", func() {
			signal := &types.NormalizedSignal{
				Fingerprint: "abc123",
				SignalName:   "OOMKilled",
				Severity:    "", // Empty
			}

			err := adapter.Validate(signal)

			Expect(err).To(HaveOccurred(),
				"BR-GATEWAY-181: Empty severity rejected - downstream policy requires value")
			Expect(err.Error()).To(ContainSubstring("severity"),
				"Clear error: severity required")
		})
	})
})

// ============================================================================
// BUSINESS OUTCOME TESTS: K8s Event Parsing for Remediation Targeting
// ============================================================================
//
// BR-GATEWAY-002: K8s Events must be parsed to extract resource information
//
// BUSINESS VALUE:
// - Resource Kind enables RO to select appropriate workflow
// - Resource Name enables WE to target correct kubectl command
// - Invalid events rejected early with clear errors
// ============================================================================

var _ = Describe("BR-GATEWAY-002: K8s Event parsing extracts remediation targeting info", func() {
	var adapter *adapters.KubernetesEventAdapter

	BeforeEach(func() {
		adapter = adapters.NewKubernetesEventAdapter()
	})

	Context("Warning events trigger remediation workflows", func() {
		It("parses Warning event and passes through event type as severity", func() {
			// BUSINESS OUTCOME: Warning event parsed → RO can select workflow
			// Authority: BR-GATEWAY-181 - Event Type passed through as-is
			payload := []byte(`{
				"involvedObject": {"kind": "Pod", "name": "payment-api-789", "namespace": "production"},
				"reason": "BackOff",
				"type": "Warning",
				"message": "Back-off restarting failed container"
			}`)

			signal, err := adapter.Parse(context.Background(), payload)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.SignalName).To(Equal("BackOff"),
				"AlertName=BackOff enables RO to select backoff-specific workflow")
			Expect(signal.Resource.Kind).To(Equal("Pod"),
				"Kind=Pod enables WE to target: kubectl delete pod")
			Expect(signal.Resource.Name).To(Equal("payment-api-789"),
				"Name enables WE to target specific pod")
			Expect(signal.Severity).To(Equal("Warning"),
				"BR-GATEWAY-181: Event Type 'Warning' passed through (no transformation)")
		})
	})

	Context("Error events trigger high-priority remediation", func() {
		It("passes through Error event type as-is (BR-GATEWAY-181)", func() {
			// BUSINESS OUTCOME: Gateway passes through raw K8s event type
			// SignalProcessing Rego policies determine severity downstream
			// Authority: BR-GATEWAY-181, DD-SEVERITY-001 v1.1
			payload := []byte(`{
				"involvedObject": {"kind": "Pod", "name": "api-server", "namespace": "prod"},
				"reason": "OOMKilled",
				"type": "Error",
				"message": "Container exceeded memory limit"
			}`)

			signal, err := adapter.Parse(context.Background(), payload)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Severity).To(Equal("Error"),
				"BR-GATEWAY-181: Event Type 'Error' passed through (no reason-based mapping)")
		})
	})

	Context("Invalid events rejected with clear errors", func() {
		It("rejects malformed JSON - helps debug webhook misconfiguration", func() {
			payload := []byte(`{invalid json`)

			_, err := adapter.Parse(context.Background(), payload)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid JSON"),
				"Clear error helps operator debug webhook configuration")
		})

		It("filters Normal events - informational, not actionable", func() {
			// BUSINESS OUTCOME: Normal events (pod created, scheduled) don't trigger remediation
			payload := []byte(`{
				"involvedObject": {"kind": "Pod", "name": "test", "namespace": "default"},
				"reason": "Created",
				"type": "Normal"
			}`)

			_, err := adapter.Parse(context.Background(), payload)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("normal events not processed"),
				"Normal events filtered - only Warning/Error trigger remediation")
		})

		It("rejects unsupported event types", func() {
			payload := []byte(`{
				"involvedObject": {"kind": "Pod", "name": "test", "namespace": "default"},
				"reason": "Unknown",
				"type": "CustomType"
			}`)

			_, err := adapter.Parse(context.Background(), payload)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported event type"),
				"Unknown event types rejected - prevents processing invalid events")
		})
	})
})
