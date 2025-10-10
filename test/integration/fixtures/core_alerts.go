<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package fixtures

import (
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// CoreTestAlerts contains the primary integration test cases
var CoreTestAlerts = []TestCase{
	{
		Name: "HighMemoryUsage",
		Alert: types.Alert{
			Name:        "HighMemoryUsage",
			Status:      "firing",
			Severity:    "warning",
			Description: "Pod webapp-123 is using 95% memory (7.6Gi/8Gi)",
			Namespace:   "production",
			Resource:    "webapp-123",
			Labels: map[string]string{
				"alertname":  "HighMemoryUsage",
				"severity":   "warning",
				"namespace":  "production",
				"pod":        "webapp-123",
				"deployment": "webapp",
				"container":  "webapp-container",
			},
			Annotations: map[string]string{
				"description": "Pod webapp-123 is using 95% memory (7.6Gi/8Gi)",
				"summary":     "High memory usage detected",
				"runbook_url": "https://wiki.kubernaut.io/runbooks/memory",
			},
			StartsAt: time.Now().Add(-5 * time.Minute),
		},
		ExpectedActions: []string{"increase_resources", "scale_deployment", "restart_pod"},
		MinConfidence:   0.7,
		Description:     "High memory usage should trigger resource or scaling actions",
	},
	{
		Name: "PodCrashLooping",
		Alert: types.Alert{
			Name:        "PodCrashLooping",
			Status:      "firing",
			Severity:    "critical",
			Description: "Pod api-server-789 has restarted 8 times in the last 5 minutes",
			Namespace:   "production",
			Resource:    "api-server-789",
			Labels: map[string]string{
				"alertname":  "PodCrashLooping",
				"severity":   "critical",
				"namespace":  "production",
				"pod":        "api-server-789",
				"deployment": "api-server",
				"reason":     "CrashLoopBackOff",
			},
			Annotations: map[string]string{
				"description":             "Pod api-server-789 has restarted 8 times in the last 5 minutes",
				"summary":                 "Pod is crash looping",
				"last_termination_reason": "OOMKilled",
				"restart_count":           "8",
				"last_exit_code":          "137",
			},
			StartsAt: time.Now().Add(-10 * time.Minute),
		},
		ExpectedActions: []string{"restart_pod", "increase_resources", "scale_deployment"},
		MinConfidence:   0.8,
		Description:     "Crash looping pods should trigger restart, resource increase, or scaling",
	},
	{
		Name: "CPUThrottling",
		Alert: types.Alert{
			Name:        "CPUThrottlingHigh",
			Status:      "firing",
			Severity:    "warning",
			Description: "Pod database-456 is experiencing high CPU throttling (45%)",
			Namespace:   "production",
			Resource:    "database-456",
			Labels: map[string]string{
				"alertname":  "CPUThrottlingHigh",
				"severity":   "warning",
				"namespace":  "production",
				"pod":        "database-456",
				"deployment": "database",
				"container":  "postgres",
			},
			Annotations: map[string]string{
				"description":           "Pod database-456 is experiencing high CPU throttling (45%)",
				"summary":               "High CPU throttling detected",
				"throttling_percentage": "45",
				"cpu_limit":             "1000m",
				"cpu_usage":             "950m",
			},
			StartsAt: time.Now().Add(-15 * time.Minute),
		},
		ExpectedActions: []string{"increase_resources", "scale_deployment"},
		MinConfidence:   0.75,
		Description:     "CPU throttling should trigger resource increase or scaling",
	},
	{
		Name: "DeploymentReplicasMismatch",
		Alert: types.Alert{
			Name:        "DeploymentReplicasMismatch",
			Status:      "firing",
			Severity:    "warning",
			Description: "Deployment frontend has 2 ready replicas but expects 5",
			Namespace:   "production",
			Resource:    "frontend",
			Labels: map[string]string{
				"alertname":  "DeploymentReplicasMismatch",
				"severity":   "warning",
				"namespace":  "production",
				"deployment": "frontend",
			},
			Annotations: map[string]string{
				"description":      "Deployment frontend has 2 ready replicas but expects 5",
				"summary":          "Deployment replicas mismatch",
				"ready_replicas":   "2",
				"desired_replicas": "5",
			},
			StartsAt: time.Now().Add(-3 * time.Minute),
		},
		ExpectedActions: []string{"restart_pod", "scale_deployment", "notify_only"},
		MinConfidence:   0.6,
		Description:     "Replica mismatch may require restart or manual intervention",
	},
	{
		Name: "LowSeverityDiskSpace",
		Alert: types.Alert{
			Name:        "DiskSpaceWarning",
			Status:      "firing",
			Severity:    "info",
			Description: "Disk space usage at 60% on storage pod",
			Namespace:   "development",
			Resource:    "storage-pod-123",
			Labels: map[string]string{
				"alertname":  "DiskSpaceWarning",
				"severity":   "info",
				"namespace":  "development",
				"pod":        "storage-pod-123",
				"mountpoint": "/data",
			},
			Annotations: map[string]string{
				"description":     "Disk space usage at 60% on storage pod",
				"summary":         "Disk space warning",
				"usage_percent":   "60",
				"available_space": "4GB",
			},
			StartsAt: time.Now().Add(-20 * time.Minute),
		},
		ExpectedActions: []string{"notify_only", "expand_pvc"},
		MinConfidence:   0.5,
		Description:     "Low severity disk space alerts should result in notification or storage expansion",
	},
	{
		Name: "NetworkConnectivityIssue",
		Alert: types.Alert{
			Name:        "PodNetworkUnavailable",
			Status:      "firing",
			Severity:    "critical",
			Description: "Pod backend-service cannot reach external dependencies",
			Namespace:   "production",
			Resource:    "backend-service-789",
			Labels: map[string]string{
				"alertname": "PodNetworkUnavailable",
				"severity":  "critical",
				"namespace": "production",
				"pod":       "backend-service-789",
				"service":   "backend",
			},
			Annotations: map[string]string{
				"description":  "Pod backend-service cannot reach external dependencies",
				"summary":      "Network connectivity issues detected",
				"target_url":   "https://api.external.com",
				"error_rate":   "100%",
				"last_success": "30m ago",
			},
			StartsAt: time.Now().Add(-30 * time.Minute),
		},
		ExpectedActions: []string{"restart_pod", "notify_only"},
		MinConfidence:   0.7,
		Description:     "Network issues may require restart or manual investigation",
	},
	{
		Name: "TestConnectivity",
		Alert: types.Alert{
			Name:        "TestConnectivity",
			Status:      "firing",
			Severity:    "info",
			Description: "Test alert for connectivity validation",
			Namespace:   "test",
			Resource:    "test-pod",
			Labels: map[string]string{
				"alertname": "TestConnectivity",
				"severity":  "info",
				"namespace": "test",
				"test_type": "connectivity",
			},
			Annotations: map[string]string{
				"description": "Test alert for connectivity validation",
				"summary":     "Connectivity test",
				"test_id":     "integration-001",
			},
			StartsAt: time.Now(),
		},
		ExpectedActions: []string{"notify_only", "restart_pod", "scale_deployment", "increase_resources"},
		MinConfidence:   0.0, // Accept any confidence for connectivity test
		Description:     "Simple test alert to validate basic functionality",
	},
}
