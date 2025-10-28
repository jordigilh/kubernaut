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

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
)

// TDD Principle: Test BUSINESS REQUIREMENTS, not implementation
// BR-GATEWAY-002: Parse Prometheus AlertManager webhook payloads

var _ = Describe("BR-GATEWAY-002: Prometheus Adapter - Parse AlertManager Webhooks", func() {
	var (
		adapter *adapters.PrometheusAdapter
		ctx     context.Context
	)

	BeforeEach(func() {
		adapter = adapters.NewPrometheusAdapter()
		ctx = context.Background()
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-006: Fingerprint Generation Algorithm (Unit Test - 70% Tier)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	//
	// UNIT TEST FOCUS: Test business logic (fingerprint algorithm, normalization rules)
	// NOT struct field extraction (that's implementation detail)
	//
	// These tests are part of the 70%+ unit tier coverage
	// Integration tests (>50% tier) test the complete flow (webhook → CRD + Redis)
	// Defense-in-Depth: Same BRs tested at multiple levels
	//
	// See: 03-testing-strategy.mdc for tier coverage explanation

	Context("BR-GATEWAY-006: Fingerprint Generation Algorithm", func() {
		It("generates consistent SHA256 fingerprint for identical alerts", func() {
			// BR-GATEWAY-006: Fingerprint consistency enables deduplication
			// BUSINESS LOGIC: Same alert → Same fingerprint (deterministic hashing)
			// Unit Test (70% tier): Tests algorithm logic, not complete flow

			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "HighMemoryUsage",
						"namespace": "production",
						"pod": "api-server-1"
					}
				}]
			}`)

			// Parse same payload twice
			signal1, err1 := adapter.Parse(ctx, payload)
			signal2, err2 := adapter.Parse(ctx, payload)

			Expect(err1).NotTo(HaveOccurred())
			Expect(err2).NotTo(HaveOccurred())

			// BUSINESS RULE: Identical alerts must produce identical fingerprints
			Expect(signal1.Fingerprint).To(Equal(signal2.Fingerprint),
				"Deduplication requires consistent fingerprints for identical alerts")

			// BUSINESS RULE: SHA256 produces 64-character hex string
			Expect(len(signal1.Fingerprint)).To(Equal(64),
				"SHA256 fingerprint must be 64 hex characters")
			Expect(signal1.Fingerprint).To(MatchRegexp("^[a-f0-9]{64}$"),
				"Fingerprint must be valid SHA256 hex string")
		})

		It("generates different fingerprints for different alerts", func() {
			// BR-GATEWAY-006: Fingerprint uniqueness enables alert differentiation
			// BUSINESS LOGIC: Different alerts → Different fingerprints
			// Unit Test (70% tier): Tests algorithm distinguishes different inputs

			payload1 := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "HighMemoryUsage",
						"namespace": "production",
						"pod": "api-server-1"
					}
				}]
			}`)

			payload2 := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "HighCPUUsage",
						"namespace": "production",
						"pod": "api-server-1"
					}
				}]
			}`)

			signal1, err1 := adapter.Parse(ctx, payload1)
			signal2, err2 := adapter.Parse(ctx, payload2)

			Expect(err1).NotTo(HaveOccurred())
			Expect(err2).NotTo(HaveOccurred())

			// BUSINESS RULE: Different alerts must produce different fingerprints
			Expect(signal1.Fingerprint).NotTo(Equal(signal2.Fingerprint),
				"Different alerts must be distinguishable for deduplication")
		})

		It("generates different fingerprints for same alert in different namespaces", func() {
			// BR-GATEWAY-006: Namespace is part of fingerprint (namespace-scoped deduplication)
			// BUSINESS LOGIC: Same alert name + different namespace → Different fingerprints

			payload1 := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "HighMemoryUsage",
						"namespace": "production",
						"pod": "api-server-1"
					}
				}]
			}`)

			payload2 := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "HighMemoryUsage",
						"namespace": "staging",
						"pod": "api-server-1"
					}
				}]
			}`)

			signal1, _ := adapter.Parse(ctx, payload1)
			signal2, _ := adapter.Parse(ctx, payload2)

			// BUSINESS RULE: Namespace-scoped deduplication
			Expect(signal1.Fingerprint).NotTo(Equal(signal2.Fingerprint),
				"Same alert in different namespaces should be treated as different alerts")
		})
	})

	Context("BR-GATEWAY-003: Signal Normalization Rules", func() {
		It("normalizes Prometheus alert to standard format for downstream processing", func() {
			// BR-GATEWAY-003: Normalization enables consistent processing
			// BUSINESS LOGIC: Prometheus format → Unified NormalizedSignal format
			// Unit Test (70% tier): Tests normalization rules

			payload := []byte(`{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "HighMemoryUsage",
						"severity": "critical",
						"namespace": "production",
						"pod": "payment-api-123"
					},
					"annotations": {
						"summary": "Pod memory usage at 95%"
					},
					"startsAt": "2025-10-22T10:00:00Z"
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).NotTo(HaveOccurred())

			// BUSINESS RULE: Required fields must be populated
			Expect(signal.AlertName).NotTo(BeEmpty(),
				"Alert name required for AI analysis")
			Expect(signal.Severity).To(BeElementOf([]string{"critical", "warning", "info"}),
				"Severity must be normalized to standard values")
			Expect(signal.Namespace).NotTo(BeEmpty(),
				"Namespace required for environment classification")
			Expect(signal.FiringTime).NotTo(BeZero(),
				"Timestamp required for timeline analysis")
			Expect(signal.SourceType).To(Equal("prometheus-alert"),
				"Source type distinguishes Prometheus from K8s events")
		})

		It("preserves raw payload for audit trail", func() {
			// BR-GATEWAY-003: Audit trail requires original payload
			// BUSINESS LOGIC: Raw payload preserved for compliance/debugging

			payload := []byte(`{"alerts": [{"labels": {"alertname": "Test", "namespace": "prod"}}]}`)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).NotTo(HaveOccurred())

			// BUSINESS RULE: Original payload must be preserved byte-for-byte
			Expect([]byte(signal.RawPayload)).To(Equal(payload),
				"Raw payload required for audit trail and debugging")
		})

		It("processes only first alert from multi-alert webhook", func() {
			// BR-GATEWAY-003: Single-alert processing simplifies deduplication
			// BUSINESS LOGIC: AlertManager sends multiple alerts → Process one at a time

			payload := []byte(`{
				"alerts": [
					{
						"labels": {
							"alertname": "FirstAlert",
							"namespace": "prod",
							"pod": "pod-1"
						}
					},
					{
						"labels": {
							"alertname": "SecondAlert",
							"namespace": "prod",
							"pod": "pod-2"
						}
					}
				]
			}`)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).NotTo(HaveOccurred())

			// BUSINESS RULE: First alert processed (server iterates for remaining)
			Expect(signal.AlertName).To(Equal("FirstAlert"),
				"Adapter processes one alert at a time for simpler deduplication")
		})
	})

	Context("BR-GATEWAY-006: Signal Normalization", func() {
		It("should include adapter source in normalized signal", func() {
			// Business Requirement: Track signal source for observability
			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "Test",
						"namespace": "prod"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Source).To(Equal("prometheus-adapter"), "BR-006: Must identify signal source")
		})

		It("should normalize resource identification across sources", func() {
			// Business Requirement: Consistent resource format for all adapters
			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "Test",
						"namespace": "production",
						"pod": "api-server-1"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Namespace).To(Equal("production"))
			Expect(signal.Resource.Kind).To(Equal("Pod"))
			Expect(signal.Resource.Name).To(Equal("api-server-1"))
			// BR-006: Normalized format enables cross-source deduplication
		})
	})
})
