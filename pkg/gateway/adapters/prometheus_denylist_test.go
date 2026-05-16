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
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
)

var _ = Describe("Prometheus Reserved Label Denylist (#1045)", func() {

	var (
		registry *adapters.APIResourceRegistry
		adapter  *adapters.PrometheusAdapter
		ctx      context.Context
	)

	BeforeEach(func() {
		fd := newFakeDiscovery(standardResources())
		var err error
		registry, err = adapters.NewAPIResourceRegistry(fd)
		Expect(err).ToNot(HaveOccurred())

		adapter = adapters.NewPrometheusAdapter(nil, registry)
		ctx = context.Background()
	})

	makePayload := func(labels map[string]interface{}) []byte {
		payload := map[string]interface{}{
			"alerts": []map[string]interface{}{
				{
					"labels":      labels,
					"annotations": map[string]interface{}{"summary": "test alert"},
					"status":      "firing",
					"startsAt":    time.Now().Format(time.RFC3339),
					"fingerprint": "test-fp-1045",
				},
			},
		}
		b, _ := json.Marshal(payload)
		return b
	}

	// =========================================================================
	// Core Denylist Enforcement (UT-GW-1045-001..005)
	// =========================================================================
	Context("Core Denylist Enforcement", func() {

		It("UT-GW-1045-001: job label (scrape target) does not resolve as K8s Job", func() {
			signal, err := adapter.Parse(ctx, makePayload(map[string]interface{}{
				"alertname": "KubePodCrashLooping",
				"namespace": "production",
				"job":       "kube-state-metrics",
				"pod":       "worker-abc",
			}))
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Pod"),
				"BR-GATEWAY-184 #1045: job label must be excluded; pod is the correct target")
			Expect(signal.Resource.Name).To(Equal("worker-abc"))
		})

		It("UT-GW-1045-002: service label (ServiceMonitor target) does not resolve as K8s Service", func() {
			signal, err := adapter.Parse(ctx, makePayload(map[string]interface{}{
				"alertname": "KubePodCrashLooping",
				"namespace": "monitoring",
				"service":   "kube-prometheus-stack-kube-state-metrics",
				"pod":       "api-server-6b9f4d7c88-x2k9m",
			}))
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Pod"),
				"BR-GATEWAY-184 #1045: service label must be excluded; pod is the correct target")
			Expect(signal.Resource.Name).To(Equal("api-server-6b9f4d7c88-x2k9m"))
		})

		It("UT-GW-1045-003: instance label (scrape endpoint) does not generate a candidate", func() {
			signal, err := adapter.Parse(ctx, makePayload(map[string]interface{}{
				"alertname": "TargetDown",
				"namespace": "monitoring",
				"instance":  "10-0-1-45",
				"pod":       "api-server-6b9f4d7c88-x2k9m",
			}))
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Pod"),
				"BR-GATEWAY-184 #1045: instance label must be excluded")
			Expect(signal.Resource.Name).To(Equal("api-server-6b9f4d7c88-x2k9m"))
		})

		It("UT-GW-1045-004: endpoint label (port name) does not generate a candidate", func() {
			signal, err := adapter.Parse(ctx, makePayload(map[string]interface{}{
				"alertname": "TargetDown",
				"namespace": "monitoring",
				"endpoint":  "http",
				"pod":       "api-server-6b9f4d7c88-x2k9m",
			}))
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Pod"),
				"BR-GATEWAY-184 #1045: endpoint label must be excluded")
		})

		It("UT-GW-1045-005: container label does not generate a candidate", func() {
			signal, err := adapter.Parse(ctx, makePayload(map[string]interface{}{
				"alertname": "ContainerOOM",
				"namespace": "production",
				"container": "payment-api",
				"pod":       "payment-api-7f86bb8877-4hv68",
			}))
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Pod"),
				"BR-GATEWAY-184 #1045: container label must be excluded")
			Expect(signal.Resource.Name).To(Equal("payment-api-7f86bb8877-4hv68"))
		})
	})

	// =========================================================================
	// Non-Reserved Labels Unaffected (UT-GW-1045-006..008)
	// =========================================================================
	Context("Non-Reserved Labels Unaffected", func() {

		It("UT-GW-1045-006: deployment label still resolves correctly after denylist", func() {
			signal, err := adapter.Parse(ctx, makePayload(map[string]interface{}{
				"alertname":  "HighMemoryUsage",
				"namespace":  "production",
				"deployment": "api-server",
			}))
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Deployment"))
			Expect(signal.Resource.Name).To(Equal("api-server"))
		})

		It("UT-GW-1045-007: job + poddisruptionbudget — PDB resolves, not Job", func() {
			signal, err := adapter.Parse(ctx, makePayload(map[string]interface{}{
				"alertname":            "PDBViolation",
				"namespace":            "production",
				"job":                  "kube-state-metrics",
				"poddisruptionbudget":  "demo-pdb",
			}))
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("PodDisruptionBudget"),
				"BR-GATEWAY-184 #1045: job excluded, PDB is the correct target")
			Expect(signal.Resource.Name).To(Equal("demo-pdb"))
		})

		It("UT-GW-1045-008: all 5 reserved + deployment — Deployment resolves correctly", func() {
			signal, err := adapter.Parse(ctx, makePayload(map[string]interface{}{
				"alertname":  "HighCPU",
				"namespace":  "production",
				"job":        "kube-state-metrics",
				"service":    "kube-prometheus-stack",
				"instance":   "10-0-1-45",
				"endpoint":   "http",
				"container":  "api-server",
				"deployment": "api-server",
			}))
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Deployment"),
				"BR-GATEWAY-184 #1045: all reserved labels excluded, deployment is the correct target")
			Expect(signal.Resource.Name).To(Equal("api-server"))
		})
	})

	// =========================================================================
	// Edge Cases (UT-GW-1045-009..011)
	// =========================================================================
	Context("Edge Cases", func() {

		It("UT-GW-1045-009: only reserved labels resolve to Unknown/unknown", func() {
			signal, err := adapter.Parse(ctx, makePayload(map[string]interface{}{
				"alertname": "Watchdog",
				"namespace": "monitoring",
				"job":       "kube-state-metrics",
				"service":   "kube-prometheus-stack",
			}))
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Unknown"),
				"BR-GATEWAY-184 #1045: only reserved labels present, no K8s resource identified")
			Expect(signal.Resource.Name).To(Equal("unknown"))
		})

		It("UT-GW-1045-010: adversarial reserved label values handled safely", func() {
			signal, err := adapter.Parse(ctx, makePayload(map[string]interface{}{
				"alertname": "Test",
				"namespace": "production",
				"job":       "",
				"service":   "../../etc/passwd",
				"pod":       "valid-pod-name",
			}))
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Pod"),
				"BR-GATEWAY-184 #1045: adversarial values in reserved labels are filtered out; pod is correct target")
			Expect(signal.Resource.Name).To(Equal("valid-pod-name"))
		})

		It("UT-GW-1045-011: reserved key casing variations don't bypass denylist", func() {
			signal, err := adapter.Parse(ctx, makePayload(map[string]interface{}{
				"alertname": "Test",
				"namespace": "production",
				"Job":       "kube-state-metrics",
				"SERVICE":   "kube-prometheus-stack",
				"Instance":  "10-0-1-45",
				"pod":       "worker-abc",
			}))
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Pod"),
				"BR-GATEWAY-184 #1045: uppercase reserved keys don't match API discovery (Prometheus labels are lowercase)")
		})
	})

	// =========================================================================
	// API Surface (UT-GW-1045-012)
	// =========================================================================
	Context("API Surface", func() {

		It("UT-GW-1045-012: PrometheusReservedLabels contains exactly 6 entries", func() {
			Expect(adapters.PrometheusReservedLabels).To(HaveLen(6))
			Expect(adapters.PrometheusReservedLabels).To(HaveKey("job"))
			Expect(adapters.PrometheusReservedLabels).To(HaveKey("service"))
			Expect(adapters.PrometheusReservedLabels).To(HaveKey("instance"))
			Expect(adapters.PrometheusReservedLabels).To(HaveKey("endpoint"))
			Expect(adapters.PrometheusReservedLabels).To(HaveKey("container"))
			Expect(adapters.PrometheusReservedLabels).To(HaveKey("namespace"))
		})
	})
})

var _ = Describe("Issue #1067: namespace label excluded from candidate scoring", func() {

	var (
		registry *adapters.APIResourceRegistry
		adapter  *adapters.PrometheusAdapter
		ctx      context.Context
	)

	BeforeEach(func() {
		fd := newFakeDiscovery(standardResources(), namespaceResource())
		var err error
		registry, err = adapters.NewAPIResourceRegistry(fd)
		Expect(err).ToNot(HaveOccurred())

		adapter = adapters.NewPrometheusAdapter(nil, registry)
		ctx = context.Background()
	})

	makePayload := func(labels map[string]interface{}) []byte {
		payload := map[string]interface{}{
			"alerts": []map[string]interface{}{
				{
					"labels":      labels,
					"annotations": map[string]interface{}{"summary": "test alert"},
					"status":      "firing",
					"startsAt":    "2026-05-09T00:00:00Z",
					"fingerprint": "test-fp-1067",
				},
			},
		}
		b, _ := json.Marshal(payload)
		return b
	}

	It("UT-GW-1067-001: namespace label does not resolve as K8s Namespace", func() {
		signal, err := adapter.Parse(ctx, makePayload(map[string]interface{}{
			"alertname": "KubePodCrashLooping",
			"namespace": "demo-alert-storm",
			"pod":       "crashing-pod-abc",
		}))
		Expect(err).ToNot(HaveOccurred())
		Expect(signal.Resource.Kind).To(Equal("Pod"),
			"BR-GATEWAY-004 #1067: namespace label is metadata, not a resource identifier; pod is the correct target")
		Expect(signal.Resource.Name).To(Equal("crashing-pod-abc"))
	})

	It("UT-GW-1067-002: namespace-only labels resolve to Unknown/unknown", func() {
		signal, err := adapter.Parse(ctx, makePayload(map[string]interface{}{
			"alertname": "KubeNamespaceTerminating",
			"namespace": "production",
		}))
		Expect(err).ToNot(HaveOccurred())
		Expect(signal.Resource.Kind).To(Equal("Unknown"),
			"BR-GATEWAY-004 #1067: namespace label excluded; no other resource candidate; resolves to Unknown")
		Expect(signal.Resource.Name).To(Equal("unknown"))
	})

	It("UT-GW-1067-003: namespace + deployment labels — Deployment wins", func() {
		signal, err := adapter.Parse(ctx, makePayload(map[string]interface{}{
			"alertname":  "HighMemoryUsage",
			"namespace":  "production",
			"deployment": "api-server",
		}))
		Expect(err).ToNot(HaveOccurred())
		Expect(signal.Resource.Kind).To(Equal("Deployment"),
			"BR-GATEWAY-069 #1067: namespace excluded, deployment is the correct tier-1 target")
		Expect(signal.Resource.Name).To(Equal("api-server"))
	})

	It("UT-GW-1067-004: exported_namespace not in denylist", func() {
		signal, err := adapter.Parse(ctx, makePayload(map[string]interface{}{
			"alertname":          "KubePodCrashLooping",
			"namespace":          "monitoring",
			"exported_namespace": "production",
			"pod":                "worker-abc",
		}))
		Expect(err).ToNot(HaveOccurred())
		Expect(signal.Resource.Kind).To(Equal("Pod"),
			"BR-GATEWAY-004 #1067: exported_namespace is not a valid API singular name, does not become a candidate")
		Expect(signal.Resource.Name).To(Equal("worker-abc"))
		Expect(signal.Resource.Namespace).To(Equal("production"),
			"exported_namespace should override namespace for the signal's namespace field")
	})
})
