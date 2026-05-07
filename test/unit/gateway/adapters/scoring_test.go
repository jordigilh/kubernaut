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
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
)

var _ = Describe("Multi-Candidate Scoring (#1029)", func() {

	var registry *adapters.APIResourceRegistry

	BeforeEach(func() {
		fd := newFakeDiscovery(standardResources(), ocpResources())
		var err error
		registry, err = adapters.NewAPIResourceRegistry(fd)
		Expect(err).ToNot(HaveOccurred())
	})

	// =========================================================================
	// Tier-Based Selection (UT-GW-1029-017..019)
	// =========================================================================
	Context("Tier-Based Selection", func() {
		It("UT-GW-1029-017: Deployment (Tier 1) wins over Pod (Tier 3) when both present", func() {
			adapter := adapters.NewPrometheusAdapter(nil, registry)
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname":  "HighMemoryUsage",
							"deployment": "api-server",
							"pod":        "api-server-abc123",
							"namespace":  "production",
						},
						"annotations": map[string]interface{}{"summary": "high memory"},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fp-017",
					},
				},
			}
			payloadBytes, _ := json.Marshal(payload)
			ctx := context.Background()
			signal, err := adapter.Parse(ctx, payloadBytes)
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Deployment"))
			Expect(signal.Resource.Name).To(Equal("api-server"))
		})

		It("UT-GW-1029-018: StatefulSet (Tier 1) wins over Pod (Tier 3)", func() {
			adapter := adapters.NewPrometheusAdapter(nil, registry)
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname":   "PodCrashLooping",
							"statefulset": "mysql-primary",
							"pod":         "mysql-primary-0",
							"namespace":   "databases",
						},
						"annotations": map[string]interface{}{"summary": "crash loop"},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fp-018",
					},
				},
			}
			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(context.Background(), payloadBytes)
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("StatefulSet"))
			Expect(signal.Resource.Name).To(Equal("mysql-primary"))
		})

		It("UT-GW-1029-019: Pod resolves alone when no higher-tier present", func() {
			adapter := adapters.NewPrometheusAdapter(nil, registry)
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname": "ContainerOOM",
							"pod":       "standalone-worker-xyz",
							"namespace": "jobs",
						},
						"annotations": map[string]interface{}{"summary": "OOM killed"},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fp-019",
					},
				},
			}
			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(context.Background(), payloadBytes)
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Pod"))
			Expect(signal.Resource.Name).To(Equal("standalone-worker-xyz"))
		})
	})

	// =========================================================================
	// OpenShift CRD Scenarios (UT-GW-1029-020..021)
	// =========================================================================
	Context("OpenShift CRD Scenarios", func() {
		It("UT-GW-1029-020: BuildConfig label resolves on OpenShift cluster", func() {
			adapter := adapters.NewPrometheusAdapter(nil, registry)
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname":   "BuildFailed",
							"buildconfig": "frontend-build",
							"namespace":   "ci-cd",
						},
						"annotations": map[string]interface{}{"summary": "Build failed"},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fp-020",
					},
				},
			}
			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(context.Background(), payloadBytes)
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("BuildConfig"))
			Expect(signal.Resource.Name).To(Equal("frontend-build"))
		})

		It("UT-GW-1029-021: Route label resolves on OpenShift cluster", func() {
			adapter := adapters.NewPrometheusAdapter(nil, registry)
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname": "RouteDown",
							"route":     "api-external",
							"namespace": "ingress",
						},
						"annotations": map[string]interface{}{"summary": "Route unavailable"},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fp-021",
					},
				},
			}
			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(context.Background(), payloadBytes)
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Route"))
			Expect(signal.Resource.Name).To(Equal("api-external"))
		})
	})

	// =========================================================================
	// Namespace Extraction (UT-GW-1029-022..023)
	// =========================================================================
	Context("Namespace Extraction", func() {
		It("UT-GW-1029-022: exported_namespace takes precedence over namespace for federated Prometheus", func() {
			adapter := adapters.NewPrometheusAdapter(nil, registry)
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname":          "HighLatency",
							"deployment":         "api-server",
							"namespace":          "monitoring",
							"exported_namespace": "production",
						},
						"annotations": map[string]interface{}{"summary": "Latency spike"},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fp-022",
					},
				},
			}
			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(context.Background(), payloadBytes)
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Namespace).To(Equal("production"),
				"exported_namespace should take precedence over namespace for federated Prometheus")
		})

		It("UT-GW-1029-023: namespace used when exported_namespace absent", func() {
			adapter := adapters.NewPrometheusAdapter(nil, registry)
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname":  "PodOOM",
							"pod":        "worker-abc",
							"namespace":  "staging",
						},
						"annotations": map[string]interface{}{"summary": "OOM"},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fp-023",
					},
				},
			}
			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(context.Background(), payloadBytes)
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Namespace).To(Equal("staging"))
		})
	})

	// =========================================================================
	// Vanilla K8s — No OCP Groups (supplement)
	// =========================================================================
	Context("Vanilla K8s (no OCP groups)", func() {
		It("'buildconfig' label returns Unknown on vanilla K8s (no OCP groups)", func() {
			// Create registry with only standard resources (no OCP)
			fd := newFakeDiscovery(standardResources())
			vanillaRegistry, err := adapters.NewAPIResourceRegistry(fd)
			Expect(err).ToNot(HaveOccurred())

			adapter := adapters.NewPrometheusAdapter(nil, vanillaRegistry)
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname":   "BuildFailed",
							"buildconfig": "frontend-build",
							"namespace":   "ci-cd",
						},
						"annotations": map[string]interface{}{"summary": "Build failed"},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fp-vanilla",
					},
				},
			}
			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(context.Background(), payloadBytes)
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Unknown"),
				"Vanilla K8s doesn't have BuildConfig, so label should not match")
		})

		It("On vanilla K8s only, 'buildconfig' label on OCP does not match 'BuildConfig'", func() {
			// This is the complement: OCP registry should match
			adapter := adapters.NewPrometheusAdapter(nil, registry)
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname":   "BuildFailed",
							"buildconfig": "frontend-build",
							"namespace":   "ci-cd",
						},
						"annotations": map[string]interface{}{"summary": "Build failed"},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fp-ocp",
					},
				},
			}
			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(context.Background(), payloadBytes)
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("BuildConfig"),
				"OCP registry has BuildConfig, so label should match")
		})
	})
})
