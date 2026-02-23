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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
)

// ============================================================================
// BUSINESS OUTCOME TESTS: Resource Extraction for Workflow Selection
// ============================================================================
//
// PURPOSE: Validate BR-GATEWAY-001, BR-GATEWAY-002 - Correct resource extraction
//          enables RO to select appropriate workflows and WE to target remediation
//
// BUSINESS VALUE:
// - RO selects workflow based on resource Kind (Pod vs Deployment vs Node)
// - WE targets specific resource Name for kubectl commands
// - Multi-tenant remediation uses correct Namespace
// - Early validation prevents downstream failures
//
// NOT TESTING: Internal parsing logic, label key formats
// ============================================================================

var _ = Describe("Prometheus Adapter - Resource Extraction for Workflow Selection", func() {
	var adapter *adapters.PrometheusAdapter

	BeforeEach(func() {
		adapter = adapters.NewPrometheusAdapter()
	})

	Context("BR-GATEWAY-001: Resource Kind Extraction (Workflow Selection)", func() {
		It("extracts Pod kind for container-level remediation workflows", func() {
			// BUSINESS OUTCOME: RO selects Pod-specific workflows (restart, scale)
			// Prometheus alert with 'pod' label → Kind=Pod
			payload := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "HighMemoryUsage",
						"pod": "payment-api-789",
						"namespace": "production",
						"severity": "critical"
					},
					"annotations": {
						"summary": "Pod memory usage critical"
					}
				}]
			}`

			signal, err := adapter.Parse(context.Background(), []byte(payload))

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Pod"),
				"Kind=Pod enables RO to select pod restart/scale workflows")
			Expect(signal.Resource.Name).To(Equal("payment-api-789"),
				"Pod name enables WE to target: kubectl delete pod payment-api-789")
		})

		It("extracts Deployment kind for application-level remediation workflows", func() {
			// BUSINESS OUTCOME: RO selects Deployment workflows (rollout restart, scale replicas)
			payload := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "DeploymentUnhealthy",
						"deployment": "payment-service",
						"namespace": "production",
						"severity": "warning"
					}
				}]
			}`

			signal, err := adapter.Parse(context.Background(), []byte(payload))

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Deployment"),
				"Kind=Deployment enables RO to select deployment-specific workflows")
			Expect(signal.Resource.Name).To(Equal("payment-service"),
				"Deployment name enables WE to target: kubectl rollout restart deployment/payment-service")
		})

		It("extracts Node kind for infrastructure-level remediation workflows", func() {
			// BUSINESS OUTCOME: RO selects Node workflows (cordon, drain, replace)
			payload := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "NodeNotReady",
						"node": "worker-node-1",
						"severity": "critical"
					}
				}]
			}`

			signal, err := adapter.Parse(context.Background(), []byte(payload))

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Node"),
				"Kind=Node enables RO to select cluster infrastructure workflows")
			Expect(signal.Resource.Name).To(Equal("worker-node-1"),
				"Node name enables WE to target: kubectl cordon worker-node-1")
			// Note: extractNamespace() defaults to "default" when no namespace label present
			// This is safe behavior for cluster-scoped resources
			Expect(signal.Resource.Namespace).To(Equal("default"),
				"Nodes get default namespace (safe fallback for cluster-scoped resources)")
		})

		It("extracts StatefulSet kind for stateful application workflows", func() {
			// BUSINESS OUTCOME: RO selects StatefulSet workflows (ordered restart, PVC handling)
			payload := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "DatabaseUnhealthy",
						"statefulset": "postgres-cluster",
						"namespace": "databases",
						"severity": "critical"
					}
				}]
			}`

			signal, err := adapter.Parse(context.Background(), []byte(payload))

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("StatefulSet"),
				"Kind=StatefulSet enables RO to select ordered restart workflows (preserves data)")
		})

		It("extracts DaemonSet kind for per-node remediation workflows", func() {
			// BUSINESS OUTCOME: RO selects DaemonSet workflows (node-level remediation)
			payload := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "LogCollectorFailing",
						"daemonset": "fluentd",
						"namespace": "logging",
						"severity": "warning"
					}
				}]
			}`

			signal, err := adapter.Parse(context.Background(), []byte(payload))

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("DaemonSet"),
				"Kind=DaemonSet enables RO to select per-node remediation workflows")
		})

		It("extracts Service kind for network-level remediation workflows", func() {
			// BUSINESS OUTCOME: RO selects Service workflows (endpoint validation, DNS checks)
			payload := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "ServiceEndpointDown",
						"service": "payment-api",
						"namespace": "production",
						"severity": "critical"
					}
				}]
			}`

			signal, err := adapter.Parse(context.Background(), []byte(payload))

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Service"),
				"Kind=Service enables RO to select network troubleshooting workflows")
		})
	})

	Context("BR-GATEWAY-001: Namespace Extraction (Multi-Tenant Remediation)", func() {
		It("extracts namespace for tenant-scoped remediation", func() {
			// BUSINESS OUTCOME: Multi-tenant remediation targets correct namespace
			// WE executes: kubectl delete pod X -n production (not -n staging)
			payload := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "PodCrashLooping",
						"pod": "api-server",
						"namespace": "production",
						"severity": "critical"
					}
				}]
			}`

			signal, err := adapter.Parse(context.Background(), []byte(payload))

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Namespace).To(Equal("production"),
				"Correct namespace ensures remediation targets right tenant")
			Expect(signal.Resource.Namespace).To(Equal("production"),
				"Resource namespace enables WE to execute: kubectl -n production")
		})

		It("handles federated Prometheus with exported_namespace label", func() {
			// BUSINESS OUTCOME: Federated monitoring across clusters preserves namespace
			payload := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "PodCrashLooping",
						"pod": "api-server",
						"exported_namespace": "staging",
						"cluster": "us-west-2",
						"severity": "warning"
					}
				}]
			}`

			signal, err := adapter.Parse(context.Background(), []byte(payload))

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Namespace).To(Equal("staging"),
				"Federated Prometheus: exported_namespace fallback preserves multi-cluster context")
		})

		It("defaults to 'default' namespace when not specified", func() {
			// BUSINESS OUTCOME: Cluster-scoped alerts without namespace use safe default
			payload := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "NodeDiskPressure",
						"node": "worker-1",
						"severity": "warning"
					}
				}]
			}`

			signal, err := adapter.Parse(context.Background(), []byte(payload))

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Namespace).To(Equal("default"),
				"Safe default namespace prevents remediation errors")
		})
	})

	Context("BR-GATEWAY-184: Priority-Based Resource Extraction", func() {
		It("prioritizes Deployment over Pod when both labels present", func() {
			// BR-GATEWAY-184: Deployment takes priority over Pod because
			// kube-state-metrics alerts inject "pod" as the metrics exporter,
			// not the affected resource. The LLM traces to pods via the Deployment.
			payload := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "ContainerCrashLooping",
						"pod": "payment-api-789",
						"deployment": "payment-api",
						"namespace": "production",
						"severity": "critical"
					}
				}]
			}`

			signal, err := adapter.Parse(context.Background(), []byte(payload))

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Deployment"),
				"BR-GATEWAY-184: Deployment takes priority over pod to avoid kube-state-metrics misidentification")
			Expect(signal.Resource.Name).To(Equal("payment-api"),
				"BR-GATEWAY-184: Resource name from deployment label, not pod label")
		})

		It("falls back to Deployment when only deployment label present", func() {
			// BUSINESS OUTCOME: Deployment-level remediation when pod not specified
			payload := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "DeploymentUnhealthy",
						"deployment": "payment-api",
						"namespace": "production",
						"severity": "warning"
					}
				}]
			}`

			signal, err := adapter.Parse(context.Background(), []byte(payload))

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Deployment"),
				"Deployment selected when more specific resource not available")
		})
	})
})

var _ = Describe("Kubernetes Event Adapter - Resource Extraction Business Outcomes", func() {
	var adapter *adapters.KubernetesEventAdapter

	BeforeEach(func() {
		adapter = adapters.NewKubernetesEventAdapter()
	})

	Context("BR-GATEWAY-002: K8s Event Resource Extraction", func() {
		It("extracts resource info from InvolvedObject for workflow selection", func() {
			// BUSINESS OUTCOME: K8s Events provide structured resource info
			// InvolvedObject contains Kind, Name, Namespace directly
			eventPayload := map[string]interface{}{
				"involvedObject": map[string]interface{}{
					"kind":      "Pod",
					"name":      "payment-pod-xyz",
					"namespace": "production",
				},
				"reason":  "OOMKilled",
				"message": "Container exceeded memory limit",
				"type":    "Error",
			}

			payload, _ := json.Marshal(eventPayload)
			signal, err := adapter.Parse(context.Background(), payload)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Pod"),
				"K8s Event: InvolvedObject.Kind enables RO workflow selection")
			Expect(signal.Resource.Name).To(Equal("payment-pod-xyz"),
				"K8s Event: InvolvedObject.Name enables WE targeting")
			Expect(signal.Resource.Namespace).To(Equal("production"),
				"K8s Event: InvolvedObject.Namespace enables tenant isolation")
		})

		It("handles cluster-scoped resources from K8s Events", func() {
			// BUSINESS OUTCOME: Node events trigger infrastructure workflows
			eventPayload := map[string]interface{}{
				"involvedObject": map[string]interface{}{
					"kind":      "Node",
					"name":      "worker-node-1",
					"namespace": "", // Cluster-scoped - no namespace
				},
				"reason":  "NodeNotReady",
				"message": "Node is not ready",
				"type":    "Error",
			}

			payload, _ := json.Marshal(eventPayload)
			signal, err := adapter.Parse(context.Background(), payload)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Resource.Kind).To(Equal("Node"),
				"Node events trigger cluster infrastructure workflows")
			Expect(signal.Resource.Name).To(Equal("worker-node-1"))
			Expect(signal.Resource.Namespace).To(BeEmpty(),
				"Cluster-scoped resources correctly have empty namespace")
		})
	})

	Context("BR-GATEWAY-181: Event Type Pass-Through (no reason-based mapping)", func() {
		It("passes through 'Error' event type as-is (OOMKilled reason not mapped)", func() {
			// BUSINESS OUTCOME: Gateway passes through raw K8s event type
			// SignalProcessing Rego policies determine severity downstream
			// Authority: BR-GATEWAY-181, DD-SEVERITY-001 v1.1
			eventPayload := map[string]interface{}{
				"involvedObject": map[string]interface{}{
					"kind":      "Pod",
					"name":      "payment-pod",
					"namespace": "production",
				},
				"reason":  "OOMKilled",
				"message": "Container exceeded memory limit",
				"type":    "Error", // Type passed through as-is
			}

			payload, _ := json.Marshal(eventPayload)
			signal, err := adapter.Parse(context.Background(), payload)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Severity).To(Equal("Error"),
				"BR-GATEWAY-181: Event Type 'Error' passed through (no reason-based mapping)")
		})

		It("passes through 'Error' event type (NodeNotReady reason ignored)", func() {
			// BUSINESS OUTCOME: Gateway extracts but does NOT interpret
			// SignalProcessing's Rego policies map node failures to severity
			// Authority: BR-GATEWAY-181
			eventPayload := map[string]interface{}{
				"involvedObject": map[string]interface{}{
					"kind": "Node",
					"name": "worker-1",
				},
				"reason": "NodeNotReady",
				"type":   "Error",
			}

			payload, _ := json.Marshal(eventPayload)
			signal, err := adapter.Parse(context.Background(), payload)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Severity).To(Equal("Error"),
				"BR-GATEWAY-181: Event Type 'Error' passed through (NodeNotReady reason not mapped)")
		})

		It("passes through 'Warning' event type (FailedScheduling reason not mapped)", func() {
			// BUSINESS OUTCOME: Gateway no longer overrides Warning with critical based on reason
			// SignalProcessing Rego policies handle scheduling failures
			// Authority: BR-GATEWAY-181
			eventPayload := map[string]interface{}{
				"involvedObject": map[string]interface{}{
					"kind":      "Pod",
					"name":      "api-pod",
					"namespace": "production",
				},
				"reason": "FailedScheduling",
				"type":   "Warning", // Passed through as-is, not overridden
			}

			payload, _ := json.Marshal(eventPayload)
			signal, err := adapter.Parse(context.Background(), payload)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Severity).To(Equal("Warning"),
				"BR-GATEWAY-181: Event Type 'Warning' passed through (no reason-based override)")
		})

		It("passes through 'Warning' event type (BackOff reason not mapped)", func() {
			// BUSINESS OUTCOME: Gateway extracts raw event type
			// SignalProcessing determines if BackOff is transient vs persistent failure
			// Authority: BR-GATEWAY-181
			eventPayload := map[string]interface{}{
				"involvedObject": map[string]interface{}{
					"kind":      "Pod",
					"name":      "api-pod",
					"namespace": "production",
				},
				"reason": "BackOff",
				"type":   "Warning",
			}

			payload, _ := json.Marshal(eventPayload)
			signal, err := adapter.Parse(context.Background(), payload)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Severity).To(Equal("Warning"),
				"BR-GATEWAY-181: Event Type 'Warning' passed through (BackOff reason not mapped)")
		})

		It("passes through 'Error' event type for any reason (no default mapping)", func() {
			// BUSINESS OUTCOME: Gateway does NOT map unknown reasons to severity
			// SignalProcessing Rego policies handle ALL severity determination
			// Authority: BR-GATEWAY-181
			eventPayload := map[string]interface{}{
				"involvedObject": map[string]interface{}{
					"kind":      "Pod",
					"name":      "test-pod",
					"namespace": "default",
				},
				"reason": "UnknownErrorReason", // Reason ignored
				"type":   "Error",              // Type passed through
			}

			payload, _ := json.Marshal(eventPayload)
			signal, err := adapter.Parse(context.Background(), payload)

			Expect(err).NotTo(HaveOccurred())
			Expect(signal.Severity).To(Equal("Error"),
				"BR-GATEWAY-181: Event Type 'Error' passed through (no default reason mapping)")
		})
	})
})
