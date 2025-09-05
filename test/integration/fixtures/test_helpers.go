package fixtures

import (
	"fmt"
	"strconv"
	"time"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
)

// PerformanceTestAlert creates an alert for performance testing
func PerformanceTestAlert(id int) types.Alert {
	return types.Alert{
		Name:        "PerformanceTest",
		Status:      "firing",
		Severity:    "info",
		Description: "Performance test alert for measuring response times",
		Namespace:   "test",
		Resource:    "perf-test-pod",
		Labels: map[string]string{
			"alertname": "PerformanceTest",
			"severity":  "info",
			"namespace": "test",
			"test_id":   strconv.Itoa(id),
			"test_type": "performance",
		},
		Annotations: map[string]string{
			"description": "Performance test alert for measuring response times",
			"summary":     "Performance test",
			"test_id":     strconv.Itoa(id),
		},
		StartsAt: time.Now(),
	}
}

// ConcurrentTestAlert creates an alert for concurrent testing
func ConcurrentTestAlert(id int) types.Alert {
	return types.Alert{
		Name:        "ConcurrentTest",
		Status:      "firing",
		Severity:    "info",
		Description: "Concurrent test alert for parallel processing validation",
		Namespace:   "test",
		Resource:    "concurrent-test-pod",
		Labels: map[string]string{
			"alertname": "ConcurrentTest",
			"severity":  "info",
			"namespace": "test",
			"test_id":   strconv.Itoa(id),
			"test_type": "concurrent",
		},
		Annotations: map[string]string{
			"description": "Concurrent test alert for parallel processing validation",
			"summary":     "Concurrent test",
			"test_id":     strconv.Itoa(id),
		},
		StartsAt: time.Now(),
	}
}

// MalformedAlert creates an intentionally malformed alert for error testing
func MalformedAlert() types.Alert {
	return types.Alert{
		Name:        "", // Missing required field
		Status:      "invalid-status",
		Severity:    "unknown-severity",
		Description: "",
		Namespace:   "",
		Resource:    "",
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
		StartsAt:    time.Time{}, // Zero time
	}
}

// ChaosEngineeringTestAlert generates chaos engineering scenarios
func ChaosEngineeringTestAlert(scenario string) types.Alert {
	scenarios := map[string]types.Alert{
		"cpu_stress": {
			Name:        "ChaosCPUStress",
			Status:      "firing",
			Severity:    "warning",
			Description: "Chaos engineering: CPU stress test injected 90% load on worker-03 for resilience testing",
			Namespace:   "chaos-engineering",
			Resource:    "cpu-stress-test",
			Labels: map[string]string{
				"alertname":   "ChaosCPUStress",
				"severity":    "warning",
				"chaos_test":  "true",
				"experiment":  "cpu_stress",
				"target_node": "worker-03",
			},
			Annotations: map[string]string{
				"description":     "Chaos engineering: CPU stress test injected 90% load on worker-03 for resilience testing",
				"summary":         "Chaos experiment: CPU stress test",
				"stress_level":    "90%",
				"duration":        "10m",
				"experiment_id":   fmt.Sprintf("chaos-cpu-%d", time.Now().Unix()),
				"expected_impact": "application_slowdown",
			},
			StartsAt: time.Now().Add(-2 * time.Minute),
		},
		"memory_leak": {
			Name:        "ChaosMemoryLeak",
			Status:      "firing",
			Severity:    "critical",
			Description: "Chaos engineering: Memory leak simulation in analytics-worker consuming 8GB in 5 minutes",
			Namespace:   "analytics",
			Resource:    "analytics-worker-chaos",
			Labels: map[string]string{
				"alertname":  "ChaosMemoryLeak",
				"severity":   "critical",
				"chaos_test": "true",
				"experiment": "memory_leak",
			},
			Annotations: map[string]string{
				"description":   "Chaos engineering: Memory leak simulation in analytics-worker consuming 8GB in 5 minutes",
				"summary":       "Chaos experiment: Memory leak simulation",
				"leaked_memory": "8GB",
				"leak_rate":     "1.6GB/min",
				"time_to_oom":   "2m",
				"experiment_id": fmt.Sprintf("chaos-memory-%d", time.Now().Unix()),
			},
			StartsAt: time.Now().Add(-5 * time.Minute),
		},
	}

	if alert, exists := scenarios[scenario]; exists {
		return alert
	}
	return scenarios["cpu_stress"] // Default
}

// SecurityIncidentAlert generates security-focused scenarios
func SecurityIncidentAlert(incidentType string) types.Alert {
	incidents := map[string]types.Alert{
		"privilege_escalation": {
			Name:        "PrivilegeEscalationAttempt",
			Status:      "firing",
			Severity:    "critical",
			Description: "Security incident: Attempted privilege escalation detected in pod web-app-789, trying to access /etc/passwd",
			Namespace:   "production",
			Resource:    "web-app-789",
			Labels: map[string]string{
				"alertname":      "PrivilegeEscalationAttempt",
				"severity":       "critical",
				"security_event": "true",
				"attack_type":    "privilege_escalation",
				"risk_level":     "high",
			},
			Annotations: map[string]string{
				"description":        "Security incident: Attempted privilege escalation detected in pod web-app-789",
				"summary":            "Critical security incident: Privilege escalation attempt",
				"target_file":        "/etc/passwd",
				"attack_vector":      "container_escape",
				"user_id":            "www-data",
				"attempted_uid":      "0",
				"forensics_required": "true",
			},
			StartsAt: time.Now().Add(-1 * time.Minute),
		},
		"data_exfiltration": {
			Name:        "SuspiciousDataAccess",
			Status:      "firing",
			Severity:    "critical",
			Description: "Security alert: Unusual data access pattern detected - 10GB customer data accessed in 30 seconds",
			Namespace:   "production",
			Resource:    "customer-api",
			Labels: map[string]string{
				"alertname":      "SuspiciousDataAccess",
				"severity":       "critical",
				"security_event": "true",
				"data_type":      "customer_pii",
				"compliance":     "GDPR",
			},
			Annotations: map[string]string{
				"description":      "Security alert: Unusual data access pattern detected - 10GB customer data accessed in 30 seconds",
				"summary":          "Potential data exfiltration attempt",
				"data_volume":      "10GB",
				"access_time":      "30s",
				"normal_rate":      "100MB/min",
				"anomaly_factor":   "200x",
				"immediate_action": "block_api_access",
			},
			StartsAt: time.Now().Add(-30 * time.Second),
		},
	}

	if alert, exists := incidents[incidentType]; exists {
		return alert
	}
	return incidents["privilege_escalation"] // Default
}

// ResourceExhaustionAlert generates resource stress scenarios
func ResourceExhaustionAlert(resourceType string) types.Alert {
	resources := map[string]types.Alert{
		"inode_exhaustion": {
			Name:        "InodeExhaustion",
			Status:      "firing",
			Severity:    "critical",
			Description: "Inode exhaustion: Filesystem /var/lib/docker using 99.8% inodes (524280/524288), cannot create files",
			Namespace:   "kube-system",
			Resource:    "docker-storage",
			Labels: map[string]string{
				"alertname":  "InodeExhaustion",
				"severity":   "critical",
				"filesystem": "/var/lib/docker",
				"resource":   "inodes",
			},
			Annotations: map[string]string{
				"description":  "Inode exhaustion: Filesystem /var/lib/docker using 99.8% inodes",
				"summary":      "Critical filesystem inode exhaustion",
				"used_inodes":  "524280",
				"total_inodes": "524288",
				"utilization":  "99.8%",
				"impact":       "cannot_create_new_files",
				"common_cause": "many_small_files",
			},
			StartsAt: time.Now().Add(-3 * time.Minute),
		},
		"network_bandwidth": {
			Name:        "NetworkBandwidthSaturation",
			Status:      "firing",
			Severity:    "critical",
			Description: "Network bandwidth saturation: Node worker-02 using 9.8Gbps/10Gbps network capacity, packet loss detected",
			Namespace:   "kube-system",
			Resource:    "worker-02",
			Labels: map[string]string{
				"alertname": "NetworkBandwidthSaturation",
				"severity":  "critical",
				"node":      "worker-02",
				"resource":  "network_bandwidth",
			},
			Annotations: map[string]string{
				"description":    "Network bandwidth saturation: Node worker-02 using 9.8Gbps/10Gbps network capacity",
				"summary":        "Critical network bandwidth exhaustion",
				"current_usage":  "9.8Gbps",
				"total_capacity": "10Gbps",
				"utilization":    "98%",
				"packet_loss":    "5%",
				"impact":         "service_degradation",
			},
			StartsAt: time.Now().Add(-2 * time.Minute),
		},
	}

	if alert, exists := resources[resourceType]; exists {
		return alert
	}
	return resources["inode_exhaustion"] // Default
}

// CascadingFailureAlert generates complex cascading failure scenarios
func CascadingFailureAlert(scenario string) types.Alert {
	scenarios := map[string]types.Alert{
		"monitoring_cascade": {
			Name:        "MonitoringSystemCascade",
			Status:      "firing",
			Severity:    "critical",
			Description: "Monitoring cascade: Prometheus down → Alertmanager silent → HPA blind → Auto-scaling failed → Performance degraded",
			Namespace:   "monitoring",
			Resource:    "prometheus-server",
			Labels: map[string]string{
				"alertname":    "MonitoringSystemCascade",
				"severity":     "critical",
				"failure_type": "monitoring_cascade",
				"impact_scope": "cluster_wide",
			},
			Annotations: map[string]string{
				"description":         "Monitoring cascade: Prometheus down → Alertmanager silent → HPA blind → Auto-scaling failed → Performance degraded",
				"summary":             "Critical monitoring system cascade failure",
				"prometheus_status":   "down",
				"alertmanager_status": "silent",
				"hpa_status":          "metrics_unavailable",
				"autoscaling_status":  "failed",
				"affected_services":   "all_monitored_services",
				"estimated_impact":    "blind_operations",
			},
			StartsAt: time.Now().Add(-8 * time.Minute),
		},
		"storage_cascade": {
			Name:        "StorageSystemCascade",
			Status:      "firing",
			Severity:    "critical",
			Description: "Storage cascade: SAN failure → PV unavailable → StatefulSets stuck → Database down → 20 services failing",
			Namespace:   "storage-system",
			Resource:    "storage-controller",
			Labels: map[string]string{
				"alertname":    "StorageSystemCascade",
				"severity":     "critical",
				"failure_type": "storage_cascade",
				"root_cause":   "san_failure",
			},
			Annotations: map[string]string{
				"description":         "Storage cascade: SAN failure → PV unavailable → StatefulSets stuck → Database down → 20 services failing",
				"summary":             "Critical storage system cascade failure",
				"san_status":          "failed",
				"pv_status":           "unavailable",
				"statefulsets_status": "pending",
				"database_status":     "down",
				"failing_services":    "20",
				"estimated_recovery":  "2-4_hours",
			},
			StartsAt: time.Now().Add(-10 * time.Minute),
		},
	}

	if alert, exists := scenarios[scenario]; exists {
		return alert
	}
	return scenarios["monitoring_cascade"] // Default
}
