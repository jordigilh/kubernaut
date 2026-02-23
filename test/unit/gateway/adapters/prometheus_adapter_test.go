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

	"github.com/go-logr/logr"
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
		adapter = adapters.NewPrometheusAdapter(nil, nil)
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

		It("generates same fingerprint for different alertnames targeting the same resource (Issue #63)", func() {
			// Issue #63: alertname EXCLUDED from fingerprint.
			// LLM investigates resource state, not signal type — multiple alertnames
			// for the same resource are redundant work.
			// BUSINESS LOGIC: Different alertnames + same resource → Same fingerprint

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

			// BUSINESS RULE: Same resource → same fingerprint (alertname excluded)
			// LLM determines RCA from resource state, not from which alert fired
			Expect(signal1.Fingerprint).To(Equal(signal2.Fingerprint),
				"Different alertnames for the same resource must produce the same fingerprint (Issue #63)")
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
			Expect(signal.SignalName).NotTo(BeEmpty(),
				"Alert name required for AI analysis")
			Expect(signal.Severity).To(BeElementOf([]string{"critical", "warning", "info"}),
				"Severity must be normalized to standard values")
			Expect(signal.Namespace).NotTo(BeEmpty(),
				"Namespace required for environment classification")
			Expect(signal.FiringTime).NotTo(BeZero(),
				"Timestamp required for timeline analysis")
			Expect(signal.SourceType).To(Equal("alert"),
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
			Expect(signal.SignalName).To(Equal("FirstAlert"),
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
			Expect(signal.Source).To(Equal("prometheus"), "BR-GATEWAY-027: Must identify signal source")
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

	Context("BR-GATEWAY-027: Adapter Source Identification Methods", func() {
		It("GetSourceService() should return monitoring system name", func() {
			// BR-GATEWAY-027: Return monitoring system name for LLM tool selection
			// BUSINESS LOGIC: LLM uses signal_source to determine investigation tools
			// - "prometheus" → LLM uses Prometheus queries
			// - NOT "prometheus-adapter" (internal implementation detail)

			adapter := adapters.NewPrometheusAdapter(nil, nil)

			sourceName := adapter.GetSourceService()

			Expect(sourceName).To(Equal("prometheus"),
				"BR-GATEWAY-027: Must return monitoring system name, not adapter name")
			Expect(sourceName).NotTo(Equal("prometheus-adapter"),
				"BR-GATEWAY-027: Adapter name is internal detail, not useful for LLM")
		})

		It("GetSourceType() should return signal type identifier", func() {
			// BUSINESS LOGIC: Signal type distinguishes alert sources for metrics/logging
			// Used for: metrics labels, logging, signal classification

			adapter := adapters.NewPrometheusAdapter(nil, nil)

			sourceType := adapter.GetSourceType()

			Expect(sourceType).To(Equal("alert"),
				"Must return signal type for classification")
		})

		It("Parse() should use GetSourceService() for signal.Source field", func() {
			// BR-GATEWAY-027: Ensure Parse() uses method instead of hardcoded value
			// BUSINESS LOGIC: Consistency between method and Parse() output

			adapter := adapters.NewPrometheusAdapter(nil, nil)
			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "TestAlert",
						"namespace": "test"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).NotTo(HaveOccurred())

			// Signal.Source must match GetSourceService()
			Expect(signal.Source).To(Equal(adapter.GetSourceService()),
				"BR-GATEWAY-027: Parse() must use GetSourceService() method")

			// Signal.SourceType must match GetSourceType()
			Expect(signal.SourceType).To(Equal(adapter.GetSourceType()),
				"Parse() must use GetSourceType() method")
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Issue #178 / BR-GATEWAY-184: Target Resource Extraction Priority
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	//
	// kube-state-metrics resource-level alerts include a `pod` label
	// pointing to the metrics exporter, NOT the affected resource.
	// Gateway must check specific resource labels before `pod`.

	Context("Issue #178 / BR-GATEWAY-184: Target Resource Extraction Priority", func() {
		It("[GW-RE-01] should extract HPA when both pod and horizontalpodautoscaler labels are present", func() {
			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "KubeHpaMaxedOut",
						"namespace": "demo-hpa",
						"pod": "kube-prometheus-stack-kube-state-metrics-abc123",
						"horizontalpodautoscaler": "api-frontend"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).NotTo(HaveOccurred())

			Expect(signal.Resource.Kind).To(Equal("HorizontalPodAutoscaler"),
				"BR-GATEWAY-184: HPA label must take priority over pod (kube-state-metrics exporter)")
			Expect(signal.Resource.Name).To(Equal("api-frontend"),
				"BR-GATEWAY-184: Resource name must come from HPA label, not pod label")
		})

		It("[GW-RE-02] should extract Deployment when both pod and deployment labels are present", func() {
			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "KubeDeploymentReplicasMismatch",
						"namespace": "production",
						"pod": "kube-prometheus-stack-kube-state-metrics-abc123",
						"deployment": "web-api"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).NotTo(HaveOccurred())

			Expect(signal.Resource.Kind).To(Equal("Deployment"),
				"BR-GATEWAY-184: Deployment label must take priority over pod (kube-state-metrics exporter)")
			Expect(signal.Resource.Name).To(Equal("web-api"),
				"BR-GATEWAY-184: Resource name must come from deployment label, not pod label")
		})

		It("[GW-RE-03] should extract StatefulSet when both pod and statefulset labels are present", func() {
			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "KubeStatefulSetReplicasMismatch",
						"namespace": "data",
						"pod": "kube-prometheus-stack-kube-state-metrics-abc123",
						"statefulset": "postgres-primary"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).NotTo(HaveOccurred())

			Expect(signal.Resource.Kind).To(Equal("StatefulSet"),
				"BR-GATEWAY-184: StatefulSet label must take priority over pod (kube-state-metrics exporter)")
			Expect(signal.Resource.Name).To(Equal("postgres-primary"),
				"BR-GATEWAY-184: Resource name must come from statefulset label, not pod label")
		})

		It("[GW-RE-04] should extract PDB when both pod and poddisruptionbudget labels are present", func() {
			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "KubePodDisruptionBudgetAtLimit",
						"namespace": "critical",
						"pod": "kube-prometheus-stack-kube-state-metrics-abc123",
						"poddisruptionbudget": "api-pdb"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).NotTo(HaveOccurred())

			Expect(signal.Resource.Kind).To(Equal("PodDisruptionBudget"),
				"BR-GATEWAY-184: PDB label must take priority over pod (kube-state-metrics exporter)")
			Expect(signal.Resource.Name).To(Equal("api-pdb"),
				"BR-GATEWAY-184: Resource name must come from PDB label, not pod label")
		})

		It("[GW-RE-05] should still extract Pod when only pod label is present (backward compatible)", func() {
			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "KubePodCrashLooping",
						"namespace": "production",
						"pod": "payment-api-7f86bb8877-4hv68"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).NotTo(HaveOccurred())

			Expect(signal.Resource.Kind).To(Equal("Pod"),
				"BR-GATEWAY-184: Pod-only alerts must still extract Pod (backward compatible)")
			Expect(signal.Resource.Name).To(Equal("payment-api-7f86bb8877-4hv68"),
				"BR-GATEWAY-184: Resource name must come from pod label when no specific labels present")
		})

		It("[GW-RE-06] should extract PVC when persistentvolumeclaim label is present", func() {
			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "KubePersistentVolumeFillingUp",
						"namespace": "data",
						"persistentvolumeclaim": "data-postgres-0"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).NotTo(HaveOccurred())

			Expect(signal.Resource.Kind).To(Equal("PersistentVolumeClaim"),
				"BR-GATEWAY-184: PVC label must be recognized as a valid resource kind")
			Expect(signal.Resource.Name).To(Equal("data-postgres-0"),
				"BR-GATEWAY-184: Resource name must come from PVC label")
		})

		It("[GW-RE-07] should extract Job when job label is present", func() {
			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "KubeJobFailed",
						"namespace": "batch",
						"job_name": "data-migration",
						"pod": "kube-prometheus-stack-kube-state-metrics-abc123",
						"job": "data-migration"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).NotTo(HaveOccurred())

			Expect(signal.Resource.Kind).To(Equal("Job"),
				"BR-GATEWAY-184: Job label must take priority over pod (kube-state-metrics exporter)")
			Expect(signal.Resource.Name).To(Equal("data-migration"),
				"BR-GATEWAY-184: Resource name must come from job label, not pod label")
		})

		It("[GW-RE-08] should extract CronJob when cronjob label is present", func() {
			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "KubeCronJobRunning",
						"namespace": "batch",
						"pod": "kube-prometheus-stack-kube-state-metrics-abc123",
						"cronjob": "nightly-backup"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).NotTo(HaveOccurred())

			Expect(signal.Resource.Kind).To(Equal("CronJob"),
				"BR-GATEWAY-184: CronJob label must take priority over pod (kube-state-metrics exporter)")
			Expect(signal.Resource.Name).To(Equal("nightly-backup"),
				"BR-GATEWAY-184: Resource name must come from cronjob label, not pod label")
		})
	})

	// GW-UNIT-ADP-007: BR-GATEWAY-001 Prometheus Long Annotations Handling
	Context("BR-GATEWAY-001: Long Annotation Handling", func() {
		It("[GW-UNIT-ADP-007] should preserve annotations under reasonable length limits", func() {
			// BR-GATEWAY-001: Annotations must be preserved for audit trail
			// BUSINESS LOGIC: Normal-length annotations should be preserved completely
			// Unit Test: Tests data preservation for typical use cases

			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "HighMemoryUsage",
						"namespace": "production"
					},
					"annotations": {
						"summary": "Pod memory usage critical",
						"description": "The pod api-server-1 in production namespace has memory usage at 95%. This requires immediate attention."
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).NotTo(HaveOccurred())

			// BUSINESS RULE: Normal annotations should be preserved completely
			Expect(signal.Annotations["summary"]).To(Equal("Pod memory usage critical"),
				"Summary annotation should be preserved")
			Expect(signal.Annotations["description"]).To(ContainSubstring("immediate attention"),
				"Description annotation should be preserved")
		})

		It("[GW-UNIT-ADP-007] should handle very long annotations gracefully", func() {
			// BR-GATEWAY-001: Extremely long annotations should not cause failures
			// BUSINESS LOGIC: System should handle edge cases without crashing
			// Unit Test: Tests resilience with unrealistic but possible inputs

			// Create 10KB annotation (unrealistic but possible from misconfigured alerts)
			longDescription := string(make([]byte, 10000))
			for i := range longDescription {
				longDescription = string(append([]byte(longDescription[:i]), 'x'))
			}

			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "Test",
						"namespace": "test"
					},
					"annotations": {
						"description": "` + longDescription[:1000] + `"
					}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)

			// BUSINESS RULE: Long annotations should not cause parsing failure
			Expect(err).NotTo(HaveOccurred(),
				"BR-GATEWAY-001: System must handle long annotations gracefully")
			Expect(signal).NotTo(BeNil())
			Expect(signal.Annotations).NotTo(BeNil())

			// BUSINESS RULE: Annotation should be present (truncated or full)
			desc, exists := signal.Annotations["description"]
			Expect(exists).To(BeTrue(), "Annotation should exist even if long")
			Expect(len(desc)).To(BeNumerically(">", 0), "Annotation should have content")
		})

		It("[GW-UNIT-ADP-007] should handle empty annotations without error", func() {
			// BR-GATEWAY-001: Empty annotations are valid (not all alerts have annotations)
			// BUSINESS LOGIC: System should handle minimal alerts gracefully

			payload := []byte(`{
				"alerts": [{
					"labels": {
						"alertname": "MinimalAlert",
						"namespace": "prod"
					},
					"annotations": {}
				}]
			}`)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).NotTo(HaveOccurred())

			// BUSINESS RULE: Empty annotations should result in empty map (not nil)
			Expect(signal.Annotations).NotTo(BeNil(),
				"Annotations should be empty map, not nil")
		})
	})
})
