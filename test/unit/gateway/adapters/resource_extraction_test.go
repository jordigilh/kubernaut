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

// ============================================================================
// BUSINESS OUTCOME TESTS: Resource Extraction for Workflow Selection
// ============================================================================
//
// PURPOSE: Validate adapters extract correct resource information for:
// - Workflow selection (RO needs correct resource Kind)
// - Remediation targeting (WE needs correct resource Name + Namespace)
// - Multi-tenant isolation (correct namespace extraction)
//
// BUSINESS VALUE:
// - RO can select appropriate workflow (Pod restart vs Deployment scale vs Node drain)
// - WE can target specific resource instance for kubectl commands
// - Multi-tenant remediation works correctly (namespace isolation)
//
// NOT TESTING: Implementation details, internal parsing logic
// ============================================================================

var _ = Describe("Prometheus Adapter - Resource Extraction for Workflow Selection", func() {
	var (
		adapter *adapters.PrometheusAdapter
		ctx     context.Context
	)

	BeforeEach(func() {
		adapter = adapters.NewPrometheusAdapter()
		ctx = context.Background()
	})

	Context("BR-GATEWAY-001: Resource Kind Extraction (Workflow Selection)", func() {
		It("extracts Pod kind for pod-specific workflows", func() {
			// BUSINESS OUTCOME: RO selects pod restart/recovery workflow
			// AlertManager sends pod alerts with 'pod' label
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname": "PodCrashLooping",
							"pod":       "payment-api-789", // Pod indicator
							"namespace": "production",
						},
						"annotations": map[string]interface{}{
							"summary": "Pod crashing",
						},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fingerprint",
					},
				},
			}

			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(ctx, payloadBytes)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Pod"),
				"Pod kind enables RO to select pod-specific workflows (restart, logs collection)")
		})

		It("extracts Deployment kind for deployment-specific workflows", func() {
			// BUSINESS OUTCOME: RO selects deployment scaling/rollback workflow
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname":  "DeploymentUnhealthy",
							"deployment": "payment-service", // Deployment indicator
							"namespace":  "production",
						},
						"annotations": map[string]interface{}{
							"summary": "Deployment unhealthy",
						},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fingerprint",
					},
				},
			}

			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(ctx, payloadBytes)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Deployment"),
				"Deployment kind enables RO to select scaling/rollback workflows")
		})

		It("extracts StatefulSet kind for stateful application workflows", func() {
			// BUSINESS OUTCOME: RO selects stateful-aware workflows
			// StatefulSets require ordered pod management
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname":   "StatefulSetDegraded",
							"statefulset": "cassandra", // StatefulSet indicator
							"namespace":   "database",
						},
						"annotations": map[string]interface{}{
							"summary": "StatefulSet degraded",
						},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fingerprint",
					},
				},
			}

			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(ctx, payloadBytes)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("StatefulSet"),
				"StatefulSet kind enables RO to select stateful-aware workflows (ordered restarts)")
		})

		It("extracts Node kind for cluster infrastructure workflows", func() {
			// BUSINESS OUTCOME: RO selects node management workflows
			// Node issues require cluster-level workflows (cordon, drain, reboot)
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname": "NodeNotReady",
							"node":      "worker-node-1", // Node indicator
						},
						"annotations": map[string]interface{}{
							"summary": "Node not ready",
						},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fingerprint",
					},
				},
			}

			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(ctx, payloadBytes)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Node"),
				"Node kind enables RO to select cluster infrastructure workflows (drain, cordon)")
		})

		It("extracts DaemonSet kind for daemon workflows", func() {
			// BUSINESS OUTCOME: RO selects daemon-specific workflows
			// DaemonSets run on every node - different remediation strategy
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname": "DaemonSetUnavailable",
							"daemonset": "node-exporter", // DaemonSet indicator
							"namespace": "monitoring",
						},
						"annotations": map[string]interface{}{
							"summary": "DaemonSet unavailable",
						},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fingerprint",
					},
				},
			}

			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(ctx, payloadBytes)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("DaemonSet"),
				"DaemonSet kind enables RO to select daemon-specific workflows")
		})
	})

	Context("BR-GATEWAY-001: Resource Name Extraction (Remediation Targeting)", func() {
		It("extracts correct pod name for targeting specific instance", func() {
			// BUSINESS OUTCOME: WE can target exact pod for kubectl commands
			// kubectl delete pod payment-api-789 -n production
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname": "PodCrashLooping",
							"pod":       "payment-api-789", // Specific pod name
							"namespace": "production",
						},
						"annotations": map[string]interface{}{
							"summary": "Pod crashing",
						},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fingerprint",
					},
				},
			}

			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(ctx, payloadBytes)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Name).To(Equal("payment-api-789"),
				"Exact resource name required for kubectl targeting: kubectl delete pod payment-api-789")
		})

		It("extracts correct deployment name for scaling operations", func() {
			// BUSINESS OUTCOME: WE can scale specific deployment
			// kubectl scale deployment payment-service --replicas=5
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname":  "DeploymentUnhealthy",
							"deployment": "payment-service", // Specific deployment name
							"namespace":  "production",
						},
						"annotations": map[string]interface{}{
							"summary": "Deployment unhealthy",
						},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fingerprint",
					},
				},
			}

			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(ctx, payloadBytes)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Name).To(Equal("payment-service"),
				"Deployment name required for scaling: kubectl scale deployment payment-service")
		})

		It("extracts correct node name for infrastructure operations", func() {
			// BUSINESS OUTCOME: WE can target specific node for drain/cordon
			// kubectl drain worker-node-1 --ignore-daemonsets
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname": "NodeNotReady",
							"node":      "worker-node-1", // Specific node name
						},
						"annotations": map[string]interface{}{
							"summary": "Node not ready",
						},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fingerprint",
					},
				},
			}

			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(ctx, payloadBytes)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Name).To(Equal("worker-node-1"),
				"Node name required for infrastructure operations: kubectl drain worker-node-1")
		})
	})

	Context("BR-GATEWAY-001: Namespace Extraction (Multi-Tenant Isolation)", func() {
		It("extracts correct namespace for multi-tenant remediation", func() {
			// BUSINESS OUTCOME: Remediation isolated to correct tenant namespace
			// Production tenant's issues don't affect staging tenant
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname": "HighMemoryUsage",
							"pod":       "api-server",
							"namespace": "production", // Multi-tenant isolation
						},
						"annotations": map[string]interface{}{
							"summary": "High memory",
						},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fingerprint",
					},
				},
			}

			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(ctx, payloadBytes)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Namespace).To(Equal("production"),
				"Namespace isolation ensures remediation affects correct tenant only")
		})

		It("extracts exported_namespace for federated Prometheus setup", func() {
			// BUSINESS OUTCOME: Multi-cluster monitoring with federated Prometheus
			// exported_namespace label used in Prometheus federation
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname":          "HighMemoryUsage",
							"pod":                "api-server",
							"exported_namespace": "staging", // Federated Prometheus label
						},
						"annotations": map[string]interface{}{
							"summary": "High memory",
						},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fingerprint",
					},
				},
			}

			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(ctx, payloadBytes)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Namespace).To(Equal("staging"),
				"exported_namespace supports federated Prometheus multi-cluster monitoring")
		})

		It("uses default namespace when not specified", func() {
			// BUSINESS OUTCOME: Cluster-scoped resources get default namespace
			// Node alerts don't have namespace - default used for CRD creation
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname": "NodeNotReady",
							"node":      "worker-node-1",
							// NO namespace label for cluster-scoped resource
						},
						"annotations": map[string]interface{}{
							"summary": "Node not ready",
						},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "test-fingerprint",
					},
				},
			}

			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(ctx, payloadBytes)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Namespace).To(Equal("default"),
				"Default namespace used for cluster-scoped resources (Node, ClusterRole)")
		})
	})
})

var _ = Describe("Kubernetes Event Adapter - Type Pass-Through (BR-GATEWAY-181)", func() {
	var (
		adapter *adapters.KubernetesEventAdapter
		ctx     context.Context
	)

	BeforeEach(func() {
		adapter = adapters.NewKubernetesEventAdapter()
		ctx = context.Background()
	})

	Context("BR-GATEWAY-181: Event Type Pass-Through (No Reason Mapping)", func() {
		It("passes through 'Warning' event type as-is (no OOMKilled mapping)", func() {
			// BUSINESS OUTCOME: Gateway passes through raw K8s event type
			// SignalProcessing Rego policies determine severity downstream
			// Authority: BR-GATEWAY-181, DD-SEVERITY-001 v1.1
			payload := map[string]interface{}{
				"reason":  "OOMKilled", // Reason NOT used for severity
				"message": "Container killed due to memory limit",
				"type":    "Warning", // Type passed through as-is
				"involvedObject": map[string]interface{}{
					"kind":      "Pod",
					"name":      "memory-hog",
					"namespace": "production",
				},
				"firstTimestamp": time.Now().Format(time.RFC3339),
			}

			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(ctx, payloadBytes)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Severity).To(Equal("Warning"),
				"BR-GATEWAY-181: Event Type 'Warning' passed through (no reason-based mapping)")
		})

		It("passes through 'Error' event type as-is (no NodeNotReady mapping)", func() {
			// BUSINESS OUTCOME: Gateway does NOT determine severity policy
			// Event type passed through → SignalProcessing determines normalized severity
			payload := map[string]interface{}{
				"reason":  "NodeNotReady", // Reason NOT used for severity
				"message": "Node is not ready",
				"type":    "Error", // Type passed through as-is
				"involvedObject": map[string]interface{}{
					"kind": "Node",
					"name": "worker-node-1",
				},
				"firstTimestamp": time.Now().Format(time.RFC3339),
			}

			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(ctx, payloadBytes)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Severity).To(Equal("Error"),
				"BR-GATEWAY-181: Event Type 'Error' passed through (no cluster-aware mapping)")
		})

	It("passes through 'Warning' event type regardless of reason (FailedScheduling)", func() {
		// BUSINESS OUTCOME: Gateway does NOT apply business rules for severity
		// FailedScheduling is serious, but SignalProcessing Rego determines final severity
		// Authority: BR-GATEWAY-181, DD-SEVERITY-001 v1.1
		payload := map[string]interface{}{
			"reason":  "FailedScheduling", // Reason does NOT override event type
			"message": "0/3 nodes available: insufficient memory",
			"type":    "Warning", // Type passed through as-is
			"involvedObject": map[string]interface{}{
				"kind":      "Pod",
				"name":      "payment-api",
				"namespace": "production",
			},
			"firstTimestamp": time.Now().Format(time.RFC3339),
		}

		payloadBytes, _ := json.Marshal(payload)
		signal, err := adapter.Parse(ctx, payloadBytes)

		Expect(err).NotTo(HaveOccurred())
		Expect(signal.Severity).To(Equal("Warning"),
			"BR-GATEWAY-181: Event Type 'Warning' passed through (reason does NOT determine severity)")
	})

	It("passes through 'Error' event type as-is (generic errors)", func() {
		// BUSINESS OUTCOME: Gateway passes through K8s event type without interpretation
		// SignalProcessing Rego policies map 'Error' → normalized severity downstream
		payload := map[string]interface{}{
			"reason":  "GenericError", // Generic error reason
			"message": "An error occurred",
			"type":    "Error", // Type passed through as-is
			"involvedObject": map[string]interface{}{
				"kind":      "Pod",
				"name":      "test-pod",
				"namespace": "default",
			},
			"firstTimestamp": time.Now().Format(time.RFC3339),
		}

		payloadBytes, _ := json.Marshal(payload)
		signal, err := adapter.Parse(ctx, payloadBytes)

		Expect(err).NotTo(HaveOccurred())
		Expect(signal.Severity).To(Equal("Error"),
			"BR-GATEWAY-181: Event Type 'Error' passed through (no default mapping)")
	})

		It("passes through 'Warning' event type as-is (BackOff reason ignored)", func() {
			// BUSINESS OUTCOME: Gateway passes through raw K8s event type
			// SignalProcessing Rego policies determine if BackOff is transient vs persistent
			// Authority: BR-GATEWAY-181, DD-SEVERITY-001 v1.1
			payload := map[string]interface{}{
				"reason":  "BackOff", // Reason NOT used for severity
				"message": "Back-off restarting failed container",
				"type":    "Warning", // Type passed through as-is
				"involvedObject": map[string]interface{}{
					"kind":      "Pod",
					"name":      "flaky-app",
					"namespace": "staging",
				},
				"firstTimestamp": time.Now().Format(time.RFC3339),
			}

			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(ctx, payloadBytes)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Severity).To(Equal("Warning"),
				"BR-GATEWAY-181: Event Type 'Warning' passed through (BackOff reason not mapped)")
		})

		It("passes through 'Warning' event type as-is (any reason)", func() {
			// BUSINESS OUTCOME: Gateway extracts raw event type, no reason-based mapping
			// SignalProcessing Rego policies handle ALL severity determination
			// Authority: BR-GATEWAY-181, DD-SEVERITY-001 v1.1
			payload := map[string]interface{}{
				"reason":  "ImagePullBackOff", // Reason ignored
				"message": "Back-off pulling image",
				"type":    "Warning", // Type passed through
				"involvedObject": map[string]interface{}{
					"kind":      "Pod",
					"name":      "test-pod",
					"namespace": "default",
				},
				"firstTimestamp": time.Now().Format(time.RFC3339),
			}

			payloadBytes, _ := json.Marshal(payload)
			signal, err := adapter.Parse(ctx, payloadBytes)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Severity).To(Equal("Warning"),
				"BR-GATEWAY-181: Event Type 'Warning' passed through (no reason-based mapping)")
		})
	})
})
