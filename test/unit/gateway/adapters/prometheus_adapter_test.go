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

	Context("when webhook contains a single firing alert", func() {
		It("should extract alert name from labels", func() {
			// Business Requirement: AlertManager webhook → NormalizedSignal with alertName
			payload := []byte(`{
				"version": "4",
				"status": "firing",
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "HighMemoryUsage",
						"namespace": "production",
						"pod": "api-server-1",
						"severity": "critical"
					},
					"annotations": {
						"summary": "Pod memory usage is at 95%"
					},
					"startsAt": "2025-10-09T10:00:00Z"
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)

			Expect(err).NotTo(HaveOccurred(), "Valid AlertManager webhook should parse successfully")
			Expect(signal.AlertName).To(Equal("HighMemoryUsage"), "BR-002: Must extract alertname from labels")
		})

		It("should extract namespace from labels", func() {
			// Business Requirement: Namespace needed for environment classification
			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "Test",
						"namespace": "production",
						"pod": "test-pod"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Namespace).To(Equal("production"), "BR-002: Must extract namespace for classification")
		})

		It("should extract severity from labels", func() {
			// Business Requirement: Severity needed for priority assignment
			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "Test",
						"namespace": "prod",
						"severity": "critical"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Severity).To(Equal("critical"), "BR-002: Must extract severity for priority")
		})

		It("should extract resource information (kind and name)", func() {
			// Business Requirement: Resource info needed for fingerprinting
			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "Test",
						"namespace": "prod",
						"pod": "api-server-1"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Pod"), "BR-002: Must identify resource kind")
			Expect(signal.Resource.Name).To(Equal("api-server-1"), "BR-002: Must extract resource name")
		})

		It("should generate unique fingerprint for deduplication", func() {
			// Business Requirement: BR-010 - Fingerprint for deduplication
			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "HighCPU",
						"namespace": "prod",
						"pod": "api-1"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Fingerprint).NotTo(BeEmpty(), "BR-010: Must generate fingerprint")
			Expect(len(signal.Fingerprint)).To(Equal(64), "BR-010: SHA256 fingerprint should be 64 chars")
		})

		It("should generate same fingerprint for identical alerts", func() {
			// Business Requirement: BR-010 - Deduplication requires consistent fingerprints
			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "Test",
						"namespace": "prod",
						"pod": "api-1"
					}
				}]
			}`)

			signal1, err := adapter.Parse(ctx, payload)
			Expect(err).NotTo(HaveOccurred())

			signal2, err := adapter.Parse(ctx, payload)
			Expect(err).NotTo(HaveOccurred())

			Expect(signal1.Fingerprint).To(Equal(signal2.Fingerprint),
				"BR-010: Identical alerts must produce same fingerprint for deduplication")
		})

		It("should preserve raw payload for audit purposes", func() {
			// Business Requirement: Audit trail needs original payload
			payload := []byte(`{"alerts": [{"labels": {"alertname": "Test", "namespace": "prod"}}]}`)

			signal, err := adapter.Parse(ctx, payload)

			Expect(err).NotTo(HaveOccurred())
			Expect([]byte(signal.RawPayload)).To(Equal(payload), "BR-002: Must preserve raw payload for audit")
		})
	})

	Context("when webhook contains multiple alerts", func() {
		It("should process only the first alert", func() {
			// Business Requirement: Each alert processed independently
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
			Expect(signal.AlertName).To(Equal("FirstAlert"),
				"BR-002: Adapter processes one alert at a time (server handles iteration)")
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
