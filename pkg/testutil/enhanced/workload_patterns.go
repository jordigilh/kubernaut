package enhanced

import (
	"k8s.io/apimachinery/pkg/api/resource"
	corev1 "k8s.io/api/core/v1"
)

// Workload pattern implementations based on real kubernaut production deployments

// getKubernautWorkloads returns workload specifications for kubernaut operator components
func (f *clusterFactory) getKubernautWorkloads() []DeploymentSpec {
	baseNamespace := "prometheus-alerts-slm"
	if !f.containsNamespace(baseNamespace) && len(f.config.Namespaces) > 0 {
		baseNamespace = f.config.Namespaces[0]
	}

	return []DeploymentSpec{
		{
			Name:      "kubernaut",
			Namespace: baseNamespace,
			Replicas:  3,
			Image:     "ghcr.io/jordigilh/kubernaut:latest",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2000m"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
			},
			Labels: map[string]string{
				"app":       "kubernaut",
				"component": "operator",
				"tier":      "control-plane",
			},
			AutoScale: true,
		},
		{
			Name:      "dynamic-toolset-server",
			Namespace: baseNamespace,
			Replicas:  2,
			Image:     "ghcr.io/jordigilh/kubernaut-toolset:latest",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1000m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
			Labels: map[string]string{
				"app":       "dynamic-toolset-server",
				"component": "api",
				"tier":      "api-layer",
			},
			AutoScale: true,
		},
		{
			Name:      "holmesgpt-api",
			Namespace: baseNamespace,
			Replicas:  2,
			Image:     "ghcr.io/jordigilh/holmesgpt-api:latest",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("300m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1500m"),
					corev1.ResourceMemory: resource.MustParse("3Gi"),
				},
			},
			Labels: map[string]string{
				"app":       "holmesgpt-api",
				"component": "ai-service",
				"tier":      "ai-layer",
			},
			AutoScale: true,
		},
		{
			Name:      "postgres-vector-db",
			Namespace: baseNamespace,
			Replicas:  1,
			Image:     "pgvector/pgvector:pg15",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2000m"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
			Labels: map[string]string{
				"app":       "postgres-vector-db",
				"component": "database",
				"tier":      "data-layer",
			},
			AutoScale: false, // Stateful, no autoscaling
		},
	}
}

// getMonitoringWorkloads returns monitoring stack workload specifications
func (f *clusterFactory) getMonitoringWorkloads() []DeploymentSpec {
	monitoringNamespace := "monitoring"
	if !f.containsNamespace(monitoringNamespace) && len(f.config.Namespaces) > 0 {
		monitoringNamespace = f.config.Namespaces[0]
	}

	return []DeploymentSpec{
		{
			Name:      "prometheus",
			Namespace: monitoringNamespace,
			Replicas:  2,
			Image:     "prom/prometheus:v2.40.7",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1000m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4000m"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
			Labels: map[string]string{
				"app":       "prometheus",
				"component": "monitoring",
				"tier":      "observability",
			},
			AutoScale: false, // Stateful monitoring, no autoscaling
		},
		{
			Name:      "grafana",
			Namespace: monitoringNamespace,
			Replicas:  1,
			Image:     "grafana/grafana:9.3.2",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1000m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
			Labels: map[string]string{
				"app":       "grafana",
				"component": "visualization",
				"tier":      "observability",
			},
			AutoScale: false,
		},
		{
			Name:      "alertmanager",
			Namespace: monitoringNamespace,
			Replicas:  2,
			Image:     "prom/alertmanager:v0.25.0",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			Labels: map[string]string{
				"app":       "alertmanager",
				"component": "alerting",
				"tier":      "observability",
			},
			AutoScale: false,
		},
		{
			Name:      "node-exporter",
			Namespace: monitoringNamespace,
			Replicas:  3, // One per node in typical setup
			Image:     "prom/node-exporter:v1.5.0",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
			Labels: map[string]string{
				"app":       "node-exporter",
				"component": "metrics",
				"tier":      "observability",
			},
			AutoScale: false, // DaemonSet-like behavior
		},
	}
}

// getAIMLWorkloads returns AI/ML workload specifications
func (f *clusterFactory) getAIMLWorkloads() []DeploymentSpec {
	aiNamespace := "ai-ml"
	if !f.containsNamespace(aiNamespace) && len(f.config.Namespaces) > 0 {
		aiNamespace = f.config.Namespaces[0]
	}

	return []DeploymentSpec{
		{
			Name:      "llm-inference-server",
			Namespace: aiNamespace,
			Replicas:  2,
			Image:     "nvidia/tritonserver:22.12-py3",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2000m"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
					"nvidia.com/gpu":      resource.MustParse("1"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("8000m"),
					corev1.ResourceMemory: resource.MustParse("32Gi"),
					"nvidia.com/gpu":      resource.MustParse("2"),
				},
			},
			Labels: map[string]string{
				"app":       "llm-inference-server",
				"component": "inference",
				"tier":      "ai-compute",
				"workload":  "gpu-intensive",
			},
			AutoScale: true,
		},
		{
			Name:      "model-training-job",
			Namespace: aiNamespace,
			Replicas:  1,
			Image:     "pytorch/pytorch:1.13.1-cuda11.6-cudnn8-runtime",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4000m"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
					"nvidia.com/gpu":      resource.MustParse("2"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("16000m"),
					corev1.ResourceMemory: resource.MustParse("64Gi"),
					"nvidia.com/gpu":      resource.MustParse("4"),
				},
			},
			Labels: map[string]string{
				"app":       "model-training-job",
				"component": "training",
				"tier":      "ai-compute",
				"workload":  "gpu-intensive",
			},
			AutoScale: false, // Training jobs typically don't autoscale
		},
		{
			Name:      "vector-embeddings-api",
			Namespace: aiNamespace,
			Replicas:  3,
			Image:     "sentence-transformers/all-MiniLM-L6-v2:latest",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1000m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4000m"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
			Labels: map[string]string{
				"app":       "vector-embeddings-api",
				"component": "embeddings",
				"tier":      "ai-api",
			},
			AutoScale: true,
		},
		{
			Name:      "jupyter-notebook-server",
			Namespace: aiNamespace,
			Replicas:  1,
			Image:     "jupyter/tensorflow-notebook:latest",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4000m"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
			},
			Labels: map[string]string{
				"app":       "jupyter-notebook-server",
				"component": "development",
				"tier":      "ai-tools",
			},
			AutoScale: false,
		},
	}
}

// getHighThroughputWorkloads returns high-performance microservices
func (f *clusterFactory) getHighThroughputWorkloads() []DeploymentSpec {
	appsNamespace := "apps"
	if !f.containsNamespace(appsNamespace) && len(f.config.Namespaces) > 0 {
		appsNamespace = f.config.Namespaces[0]
	}

	return []DeploymentSpec{
		{
			Name:      "api-gateway",
			Namespace: appsNamespace,
			Replicas:  5,
			Image:     "nginx:1.23-alpine",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1000m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			Labels: map[string]string{
				"app":       "api-gateway",
				"component": "gateway",
				"tier":      "edge",
			},
			AutoScale: true,
		},
		{
			Name:      "user-service",
			Namespace: appsNamespace,
			Replicas:  8,
			Image:     "golang:1.19-alpine",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("300m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1500m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
			Labels: map[string]string{
				"app":       "user-service",
				"component": "microservice",
				"tier":      "business-logic",
			},
			AutoScale: true,
		},
		{
			Name:      "order-processing-service",
			Namespace: appsNamespace,
			Replicas:  6,
			Image:     "openjdk:17-jre-slim",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2000m"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
			},
			Labels: map[string]string{
				"app":       "order-processing-service",
				"component": "microservice",
				"tier":      "business-logic",
			},
			AutoScale: true,
		},
		{
			Name:      "redis-cache",
			Namespace: appsNamespace,
			Replicas:  3,
			Image:     "redis:7-alpine",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
			Labels: map[string]string{
				"app":       "redis-cache",
				"component": "cache",
				"tier":      "data-layer",
			},
			AutoScale: false, // Stateful cache
		},
		{
			Name:      "message-queue",
			Namespace: appsNamespace,
			Replicas:  3,
			Image:     "rabbitmq:3.11-management-alpine",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1000m"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
			Labels: map[string]string{
				"app":       "message-queue",
				"component": "messaging",
				"tier":      "infrastructure",
			},
			AutoScale: false, // Message queue clustering requires careful setup
		},
	}
}

// getWebApplicationWorkloads returns typical web application stack
func (f *clusterFactory) getWebApplicationWorkloads() []DeploymentSpec {
	appsNamespace := "apps"
	if !f.containsNamespace(appsNamespace) && len(f.config.Namespaces) > 0 {
		appsNamespace = f.config.Namespaces[0]
	}

	return []DeploymentSpec{
		{
			Name:      "web-frontend",
			Namespace: appsNamespace,
			Replicas:  3,
			Image:     "nginx:1.23-alpine",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
			},
			Labels: map[string]string{
				"app":       "web-frontend",
				"component": "frontend",
				"tier":      "presentation",
			},
			AutoScale: true,
		},
		{
			Name:      "api-backend",
			Namespace: appsNamespace,
			Replicas:  4,
			Image:     "node:18-alpine",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1000m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			Labels: map[string]string{
				"app":       "api-backend",
				"component": "backend",
				"tier":      "business-logic",
			},
			AutoScale: true,
		},
		{
			Name:      "database",
			Namespace: appsNamespace,
			Replicas:  1,
			Image:     "postgres:15-alpine",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("300m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1500m"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
			},
			Labels: map[string]string{
				"app":       "database",
				"component": "database",
				"tier":      "data-layer",
			},
			AutoScale: false, // Stateful database
		},
		{
			Name:      "cache",
			Namespace: appsNamespace,
			Replicas:  2,
			Image:     "redis:7-alpine",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("250m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			Labels: map[string]string{
				"app":       "cache",
				"component": "cache",
				"tier":      "data-layer",
			},
			AutoScale: false,
		},
	}
}
