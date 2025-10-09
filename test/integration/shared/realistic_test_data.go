//go:build integration
// +build integration

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

package shared

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RealisticTestDataGenerator creates realistic test data for various scenarios
type RealisticTestDataGenerator struct {
	namespaces []string
	services   []string
	instances  []string
}

// NewRealisticTestDataGenerator creates a new test data generator
func NewRealisticTestDataGenerator() *RealisticTestDataGenerator {
	return &RealisticTestDataGenerator{
		namespaces: []string{"default", "kube-system", "production", "staging", "monitoring"},
		services:   []string{"web-app", "api-service", "database", "cache", "worker", "ingress", "auth-service"},
		instances:  []string{"web-app-1", "web-app-2", "api-1", "db-master", "db-replica", "redis-1", "worker-1", "worker-2"},
	}
}

// GenerateRealisticAlert creates a realistic alert based on a scenario
func (g *RealisticTestDataGenerator) GenerateRealisticAlert(alertType string) types.Alert {
	baseTime := time.Now()

	alert := types.Alert{
		Name:      alertType,
		Namespace: g.namespaces[rand.Intn(len(g.namespaces))],
		Resource:  g.instances[rand.Intn(len(g.instances))],
		Severity:  g.getSeverityForAlert(alertType),
		Labels:    make(map[string]string),
		StartsAt:  baseTime.Add(-time.Duration(rand.Intn(3600)) * time.Second), // Up to 1 hour ago
	}

	// Add realistic labels based on alert type
	switch alertType {
	case "OOMKilled":
		alert.Labels["container"] = "app-container"
		alert.Labels["pod"] = fmt.Sprintf("%s-pod-%d", alert.Resource, rand.Intn(1000))
		alert.Labels["exit_code"] = "137"

	case "HighMemoryUsage":
		alert.Labels["container"] = "app-container"
		alert.Labels["threshold"] = "85"
		alert.Labels["current_usage"] = fmt.Sprintf("%.1f", 85.0+rand.Float64()*10.0) // 85-95%

	case "HighCPUUsage":
		alert.Labels["container"] = "app-container"
		alert.Labels["threshold"] = "80"
		alert.Labels["current_usage"] = fmt.Sprintf("%.1f", 80.0+rand.Float64()*15.0) // 80-95%

	case "DiskSpaceLow":
		alert.Labels["mountpoint"] = "/var/lib/data"
		alert.Labels["threshold"] = "90"
		alert.Labels["current_usage"] = fmt.Sprintf("%.1f", 90.0+rand.Float64()*8.0) // 90-98%

	case "PodCrashLooping":
		alert.Labels["container"] = "app-container"
		alert.Labels["restart_count"] = fmt.Sprintf("%d", 5+rand.Intn(15)) // 5-20 restarts
		alert.Labels["exit_code"] = fmt.Sprintf("%d", []int{1, 2, 125, 126, 127}[rand.Intn(5)])

	case "ServiceDown":
		alert.Labels["service"] = g.services[rand.Intn(len(g.services))]
		alert.Labels["endpoint"] = "/health"
		alert.Labels["status_code"] = "500"

	case "HighNetworkLatency":
		alert.Labels["source"] = alert.Resource
		alert.Labels["destination"] = g.services[rand.Intn(len(g.services))]
		alert.Labels["latency_ms"] = fmt.Sprintf("%.0f", 1000+rand.Float64()*2000) // 1000-3000ms

	case "DatabaseConnectionsHigh":
		alert.Labels["database"] = "postgres"
		alert.Labels["max_connections"] = "100"
		alert.Labels["current_connections"] = fmt.Sprintf("%d", 85+rand.Intn(20)) // 85-105

	case "UnauthorizedAccess":
		alert.Labels["source_ip"] = g.generateRandomIP()
		alert.Labels["endpoint"] = "/api/admin"
		alert.Labels["status_code"] = "403"
		alert.Labels["user_agent"] = "curl/7.68.0"
		alert.Severity = "critical" // Security alerts are always critical

	case "SlowResponseTime":
		alert.Labels["endpoint"] = fmt.Sprintf("/api/%s", []string{"users", "orders", "products", "metrics"}[rand.Intn(4)])
		alert.Labels["response_time_ms"] = fmt.Sprintf("%.0f", 2000+rand.Float64()*3000) // 2000-5000ms
		alert.Labels["threshold_ms"] = "2000"
	}

	// Add common labels
	alert.Labels["cluster"] = "production-cluster"
	alert.Labels["datacenter"] = []string{"us-east-1", "us-west-2", "eu-west-1"}[rand.Intn(3)]

	return alert
}

// getSeverityForAlert determines appropriate severity for alert type
func (g *RealisticTestDataGenerator) getSeverityForAlert(alertType string) string {
	severityMap := map[string]string{
		"OOMKilled":               "critical",
		"HighMemoryUsage":         "warning",
		"HighCPUUsage":            "warning",
		"DiskSpaceLow":            "critical",
		"PodCrashLooping":         "critical",
		"ServiceDown":             "critical",
		"HighNetworkLatency":      "warning",
		"DatabaseConnectionsHigh": "critical",
		"UnauthorizedAccess":      "critical",
		"SlowResponseTime":        "warning",
		"LoadBalancerError":       "critical",
		"HighErrorRate":           "warning",
	}

	if severity, exists := severityMap[alertType]; exists {
		return severity
	}
	return "warning"
}

// generateRandomIP creates a realistic IP address
func (g *RealisticTestDataGenerator) generateRandomIP() string {
	return fmt.Sprintf("%d.%d.%d.%d",
		rand.Intn(255), rand.Intn(255), rand.Intn(255), rand.Intn(255))
}

// GenerateCorrelatedAlerts creates a set of alerts that are realistically correlated
func (g *RealisticTestDataGenerator) GenerateCorrelatedAlerts(scenario string) []types.Alert {
	baseTime := time.Now()
	alerts := make([]types.Alert, 0)

	switch scenario {
	case "memory_pressure_cascade":
		// Start with high memory usage
		memAlert := g.GenerateRealisticAlert("HighMemoryUsage")
		memAlert.StartsAt = baseTime.Add(-10 * time.Minute)
		alerts = append(alerts, memAlert)

		// Followed by OOM kills
		for i := 0; i < 2+rand.Intn(3); i++ {
			oomAlert := g.GenerateRealisticAlert("OOMKilled")
			oomAlert.StartsAt = baseTime.Add(-time.Duration(8-i*2) * time.Minute)
			oomAlert.Resource = memAlert.Resource   // Same resource
			oomAlert.Namespace = memAlert.Namespace // Same namespace
			alerts = append(alerts, oomAlert)
		}

		// Potentially followed by pod crash looping
		if rand.Float64() < 0.7 { // 70% chance
			crashAlert := g.GenerateRealisticAlert("PodCrashLooping")
			crashAlert.StartsAt = baseTime.Add(-3 * time.Minute)
			crashAlert.Resource = memAlert.Resource
			crashAlert.Namespace = memAlert.Namespace
			alerts = append(alerts, crashAlert)
		}

	case "service_degradation":
		// Start with slow response times
		slowAlert := g.GenerateRealisticAlert("SlowResponseTime")
		slowAlert.StartsAt = baseTime.Add(-15 * time.Minute)
		alerts = append(alerts, slowAlert)

		// Followed by high error rates
		errorAlert := g.GenerateRealisticAlert("HighErrorRate")
		errorAlert.StartsAt = baseTime.Add(-10 * time.Minute)
		errorAlert.Resource = slowAlert.Resource
		errorAlert.Namespace = slowAlert.Namespace
		alerts = append(alerts, errorAlert)

		// Potentially complete service failure
		if rand.Float64() < 0.6 { // 60% chance
			downAlert := g.GenerateRealisticAlert("ServiceDown")
			downAlert.StartsAt = baseTime.Add(-5 * time.Minute)
			downAlert.Resource = slowAlert.Resource
			downAlert.Namespace = slowAlert.Namespace
			alerts = append(alerts, downAlert)
		}

	case "resource_exhaustion":
		// High CPU usage
		cpuAlert := g.GenerateRealisticAlert("HighCPUUsage")
		cpuAlert.StartsAt = baseTime.Add(-20 * time.Minute)
		alerts = append(alerts, cpuAlert)

		// Memory pressure
		memAlert := g.GenerateRealisticAlert("HighMemoryUsage")
		memAlert.StartsAt = baseTime.Add(-18 * time.Minute)
		memAlert.Resource = cpuAlert.Resource
		memAlert.Namespace = cpuAlert.Namespace
		alerts = append(alerts, memAlert)

		// Database connection pressure
		dbAlert := g.GenerateRealisticAlert("DatabaseConnectionsHigh")
		dbAlert.StartsAt = baseTime.Add(-15 * time.Minute)
		alerts = append(alerts, dbAlert)

		// Disk space issues
		diskAlert := g.GenerateRealisticAlert("DiskSpaceLow")
		diskAlert.StartsAt = baseTime.Add(-12 * time.Minute)
		diskAlert.Resource = cpuAlert.Resource
		diskAlert.Namespace = cpuAlert.Namespace
		alerts = append(alerts, diskAlert)
	}

	return alerts
}

// GenerateRealisticPod creates a realistic pod for K8s testing
func (g *RealisticTestDataGenerator) GenerateRealisticPod(namespace, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":     "test-app",
				"version": "v1.0.0",
				"tier":    "backend",
			},
			Annotations: map[string]string{
				"prometheus.io/scrape": "true",
				"prometheus.io/port":   "8080",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app-container",
					Image: "nginx:1.21",
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
					Ports: []corev1.ContainerPort{
						{ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyAlways,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
}

// GenerateRealisticDeployment creates a realistic deployment for K8s testing
func (g *RealisticTestDataGenerator) GenerateRealisticDeployment(namespace, name string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":     name,
				"version": "v1.0.0",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":     name,
						"version": "v1.0.0",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: fmt.Sprintf("%s:latest", name),
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1000m"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
					},
				},
			},
		},
	}
}

// GetRealisticAlertTypes returns the list of supported realistic alert types
func (g *RealisticTestDataGenerator) GetRealisticAlertTypes() []string {
	return []string{
		"OOMKilled",
		"HighMemoryUsage",
		"HighCPUUsage",
		"DiskSpaceLow",
		"PodCrashLooping",
		"ServiceDown",
		"HighNetworkLatency",
		"DatabaseConnectionsHigh",
		"UnauthorizedAccess",
		"SlowResponseTime",
		"LoadBalancerError",
		"HighErrorRate",
	}
}

// GetRealisticScenarios returns available correlation scenarios
func (g *RealisticTestDataGenerator) GetRealisticScenarios() []string {
	return []string{
		"memory_pressure_cascade",
		"service_degradation",
		"resource_exhaustion",
	}
}
