package integration

import (
	"fmt"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/pkg/slm"
)

// TestCase represents a test case for alert analysis
type TestCase struct {
	Name            string
	Alert           slm.Alert
	ExpectedActions []string
	MinConfidence   float64
	Description     string
}

// IntegrationTestAlerts contains all test cases for integration testing
var IntegrationTestAlerts = []TestCase{
	{
		Name: "HighMemoryUsage",
		Alert: slm.Alert{
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
				"runbook_url": "https://wiki.example.com/runbooks/memory",
			},
			StartsAt: time.Now().Add(-5 * time.Minute),
		},
		ExpectedActions: []string{"increase_resources", "scale_deployment", "restart_pod"},
		MinConfidence:   0.7,
		Description:     "High memory usage should trigger resource or scaling actions",
	},
	{
		Name: "PodCrashLooping",
		Alert: slm.Alert{
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
		ExpectedActions: []string{"restart_pod", "increase_resources"},
		MinConfidence:   0.8,
		Description:     "Crash looping pods should trigger restart or resource increase",
	},
	{
		Name: "CPUThrottling",
		Alert: slm.Alert{
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
		Alert: slm.Alert{
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
		Alert: slm.Alert{
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
		ExpectedActions: []string{"notify_only"},
		MinConfidence:   0.5,
		Description:     "Low severity alerts should typically result in notify_only",
	},
	{
		Name: "NetworkConnectivityIssue",
		Alert: slm.Alert{
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
		Alert: slm.Alert{
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

	// === PRODUCTION EDGE CASES ===
	{
		Name: "NodeResourceExhaustion",
		Alert: slm.Alert{
			Name:        "NodeMemoryPressure",
			Status:      "firing",
			Severity:    "critical",
			Description: "Node worker-03 has 97% memory usage with 15 pods at risk of eviction",
			Namespace:   "kube-system",
			Resource:    "worker-03",
			Labels: map[string]string{
				"alertname": "NodeMemoryPressure",
				"severity":  "critical",
				"node":      "worker-03",
				"instance":  "10.0.1.23:9100",
				"job":       "node-exporter",
				"cluster":   "prod-east-1",
				"node_role": "worker",
			},
			Annotations: map[string]string{
				"description":      "Node worker-03 has 97% memory usage with 15 pods at risk of eviction",
				"summary":          "Critical node memory pressure detected",
				"memory_usage":     "97%",
				"pods_at_risk":     "15",
				"available_memory": "512Mi",
				"total_memory":     "16Gi",
				"runbook_url":      "https://wiki.example.com/runbooks/node-pressure",
			},
			StartsAt: time.Now().Add(-2 * time.Minute),
		},
		ExpectedActions: []string{"scale_deployment", "notify_only"},
		MinConfidence:   0.8,
		Description:     "Node pressure requires careful handling to avoid cascading failures",
	},
	{
		Name: "EtcdHighLatency",
		Alert: slm.Alert{
			Name:        "EtcdHighRequestLatency",
			Status:      "firing",
			Severity:    "critical",
			Description: "Etcd request latency is 2.3s (>500ms threshold) affecting API server",
			Namespace:   "kube-system",
			Resource:    "etcd-master-1",
			Labels: map[string]string{
				"alertname": "EtcdHighRequestLatency",
				"severity":  "critical",
				"instance":  "etcd-master-1",
				"job":       "etcd",
				"cluster":   "prod-east-1",
			},
			Annotations: map[string]string{
				"description": "Etcd request latency is 2.3s (>500ms threshold) affecting API server",
				"summary":     "Critical etcd performance degradation",
				"latency":     "2.3s",
				"threshold":   "500ms",
				"impact":      "API server slowdown",
			},
			StartsAt: time.Now().Add(-30 * time.Second),
		},
		ExpectedActions: []string{"notify_only"},
		MinConfidence:   0.9,
		Description:     "Etcd issues require immediate human intervention, not automated remediation",
	},
	{
		Name: "CascadingFailureScenario",
		Alert: slm.Alert{
			Name:        "ServiceUnavailable",
			Status:      "firing",
			Severity:    "critical",
			Description: "Payment service down: 5/5 replicas failing, 100% error rate, database connection timeout",
			Namespace:   "production",
			Resource:    "payment-service",
			Labels: map[string]string{
				"alertname":  "ServiceUnavailable",
				"severity":   "critical",
				"namespace":  "production",
				"service":    "payment-service",
				"deployment": "payment-api",
				"tier":       "business-critical",
			},
			Annotations: map[string]string{
				"description":      "Payment service down: 5/5 replicas failing, 100% error rate, database connection timeout",
				"summary":          "Critical business service failure",
				"error_rate":       "100%",
				"healthy_replicas": "0/5",
				"root_cause":       "database_timeout",
				"impact":           "payment_processing_down",
				"revenue_impact":   "$50000/hour",
			},
			StartsAt: time.Now().Add(-1 * time.Minute),
		},
		ExpectedActions: []string{"restart_pod", "notify_only"},
		MinConfidence:   0.8,
		Description:     "Business-critical failures may need immediate restart while investigating root cause",
	},
	{
		Name: "SecurityPodCompromise",
		Alert: slm.Alert{
			Name:        "PodSecurityViolation",
			Status:      "firing",
			Severity:    "critical",
			Description: "Pod web-frontend-456 running as root with privileged access, potential security breach",
			Namespace:   "production",
			Resource:    "web-frontend-456",
			Labels: map[string]string{
				"alertname":        "PodSecurityViolation",
				"severity":         "critical",
				"namespace":        "production",
				"pod":              "web-frontend-456",
				"deployment":       "web-frontend",
				"security_context": "privileged",
			},
			Annotations: map[string]string{
				"description":     "Pod web-frontend-456 running as root with privileged access, potential security breach",
				"summary":         "Critical security policy violation",
				"user_id":         "root",
				"privileged":      "true",
				"host_network":    "true",
				"security_risk":   "high",
				"action_required": "immediate_quarantine",
			},
			StartsAt: time.Now().Add(-30 * time.Second),
		},
		ExpectedActions: []string{"restart_pod", "notify_only"},
		MinConfidence:   0.95,
		Description:     "Security violations require immediate action but careful handling",
	},
	{
		Name: "ResourceQuotaExceeded",
		Alert: slm.Alert{
			Name:        "NamespaceResourceQuotaExceeded",
			Status:      "firing",
			Severity:    "warning",
			Description: "Namespace 'production' using 95% of CPU quota (190/200 cores), blocking new deployments",
			Namespace:   "production",
			Resource:    "production-quota",
			Labels: map[string]string{
				"alertname":  "NamespaceResourceQuotaExceeded",
				"severity":   "warning",
				"namespace":  "production",
				"resource":   "cpu",
				"quota_name": "production-quota",
			},
			Annotations: map[string]string{
				"description":         "Namespace 'production' using 95% of CPU quota (190/200 cores), blocking new deployments",
				"summary":             "Resource quota near exhaustion",
				"used_cpu":            "190 cores",
				"quota_cpu":           "200 cores",
				"usage_percent":       "95%",
				"pending_pods":        "12",
				"blocked_deployments": "payment-v2, user-service-v3",
			},
			StartsAt: time.Now().Add(-10 * time.Minute),
		},
		ExpectedActions: []string{"notify_only", "scale_deployment"},
		MinConfidence:   0.7,
		Description:     "Quota exhaustion requires careful resource management decisions",
	},
	{
		Name: "PersistentVolumeFailure",
		Alert: slm.Alert{
			Name:        "PersistentVolumeClaimPending",
			Status:      "firing",
			Severity:    "critical",
			Description: "Database PVC 'postgres-data-claim' stuck in Pending state for 15 minutes, no available storage",
			Namespace:   "database",
			Resource:    "postgres-data-claim",
			Labels: map[string]string{
				"alertname":     "PersistentVolumeClaimPending",
				"severity":      "critical",
				"namespace":     "database",
				"pvc":           "postgres-data-claim",
				"storage_class": "ssd-retain",
			},
			Annotations: map[string]string{
				"description":    "Database PVC 'postgres-data-claim' stuck in Pending state for 15 minutes, no available storage",
				"summary":        "Critical storage provisioning failure",
				"requested_size": "500Gi",
				"pending_time":   "15m",
				"reason":         "insufficient_storage",
				"impact":         "database_cannot_start",
			},
			StartsAt: time.Now().Add(-15 * time.Minute),
		},
		ExpectedActions: []string{"notify_only"},
		MinConfidence:   0.9,
		Description:     "Storage issues require infrastructure-level intervention",
	},
	{
		Name: "LivenessProbeMultipleFailures",
		Alert: slm.Alert{
			Name:        "PodLivenessProbeFailure",
			Status:      "firing",
			Severity:    "warning",
			Description: "Pod api-gateway-789 liveness probe failing for 8 minutes: HTTP 503 on /health endpoint",
			Namespace:   "production",
			Resource:    "api-gateway-789",
			Labels: map[string]string{
				"alertname":  "PodLivenessProbeFailure",
				"severity":   "warning",
				"namespace":  "production",
				"pod":        "api-gateway-789",
				"deployment": "api-gateway",
				"container":  "gateway",
			},
			Annotations: map[string]string{
				"description":      "Pod api-gateway-789 liveness probe failing for 8 minutes: HTTP 503 on /health endpoint",
				"summary":          "Pod health check failing persistently",
				"failure_count":    "24",
				"failure_duration": "8m",
				"probe_endpoint":   "/health",
				"last_response":    "503 Service Unavailable",
				"restart_count":    "0",
			},
			StartsAt: time.Now().Add(-8 * time.Minute),
		},
		ExpectedActions: []string{"restart_pod"},
		MinConfidence:   0.85,
		Description:     "Persistent liveness failures usually require pod restart",
	},
	{
		Name: "IngressControllerOverload",
		Alert: slm.Alert{
			Name:        "IngressControllerHighLatency",
			Status:      "firing",
			Severity:    "critical",
			Description: "Ingress controller nginx-ingress-001 showing 5s response times, 85% CPU usage, 10000 req/s",
			Namespace:   "ingress-nginx",
			Resource:    "nginx-ingress-001",
			Labels: map[string]string{
				"alertname":  "IngressControllerHighLatency",
				"severity":   "critical",
				"namespace":  "ingress-nginx",
				"pod":        "nginx-ingress-001",
				"deployment": "nginx-ingress",
				"component":  "ingress-controller",
			},
			Annotations: map[string]string{
				"description":        "Ingress controller nginx-ingress-001 showing 5s response times, 85% CPU usage, 10000 req/s",
				"summary":            "Critical ingress performance degradation",
				"response_time":      "5s",
				"cpu_usage":          "85%",
				"request_rate":       "10000/s",
				"error_rate":         "15%",
				"active_connections": "50000",
			},
			StartsAt: time.Now().Add(-3 * time.Minute),
		},
		ExpectedActions: []string{"scale_deployment", "restart_pod"},
		MinConfidence:   0.8,
		Description:     "Ingress overload requires scaling or restart to handle traffic",
	},
	{
		Name: "DatabaseConnectionPoolExhaustion",
		Alert: slm.Alert{
			Name:        "DatabaseConnectionPoolExhausted",
			Status:      "firing",
			Severity:    "critical",
			Description: "PostgreSQL connection pool exhausted: 500/500 connections used, 200 queries waiting",
			Namespace:   "database",
			Resource:    "postgres-primary",
			Labels: map[string]string{
				"alertname": "DatabaseConnectionPoolExhausted",
				"severity":  "critical",
				"namespace": "database",
				"pod":       "postgres-primary-0",
				"service":   "postgres",
				"role":      "primary",
			},
			Annotations: map[string]string{
				"description":        "PostgreSQL connection pool exhausted: 500/500 connections used, 200 queries waiting",
				"summary":            "Database connection pool at capacity",
				"max_connections":    "500",
				"active_connections": "500",
				"waiting_queries":    "200",
				"connection_errors":  "150/min",
				"avg_query_time":     "12s",
			},
			StartsAt: time.Now().Add(-5 * time.Minute),
		},
		ExpectedActions: []string{"restart_pod", "notify_only"},
		MinConfidence:   0.8,
		Description:     "Database connection exhaustion may require restart or configuration changes",
	},
	{
		Name: "KubernetesAPIServerDown",
		Alert: slm.Alert{
			Name:        "KubernetesAPIDown",
			Status:      "firing",
			Severity:    "critical",
			Description: "Kubernetes API server on master-2 unreachable for 90 seconds, cluster operations failing",
			Namespace:   "kube-system",
			Resource:    "kube-apiserver-master-2",
			Labels: map[string]string{
				"alertname": "KubernetesAPIDown",
				"severity":  "critical",
				"instance":  "master-2",
				"job":       "kubernetes-apiserver",
				"cluster":   "prod-east-1",
			},
			Annotations: map[string]string{
				"description":  "Kubernetes API server on master-2 unreachable for 90 seconds, cluster operations failing",
				"summary":      "Critical control plane component failure",
				"downtime":     "90s",
				"impact":       "cluster_operations_failing",
				"last_seen":    "90s ago",
				"health_check": "failed",
			},
			StartsAt: time.Now().Add(-90 * time.Second),
		},
		ExpectedActions: []string{"notify_only"},
		MinConfidence:   0.95,
		Description:     "Control plane failures require immediate human intervention",
	},
	{
		Name: "HorizontalPodAutoscalerFailure",
		Alert: slm.Alert{
			Name:        "HPAScaleFailure",
			Status:      "firing",
			Severity:    "warning",
			Description: "HPA 'webapp-hpa' unable to scale: CPU metrics unavailable for 10 minutes, stuck at 2/10 replicas",
			Namespace:   "production",
			Resource:    "webapp-hpa",
			Labels: map[string]string{
				"alertname":   "HPAScaleFailure",
				"severity":    "warning",
				"namespace":   "production",
				"hpa":         "webapp-hpa",
				"deployment":  "webapp",
				"metric_type": "cpu",
			},
			Annotations: map[string]string{
				"description":      "HPA 'webapp-hpa' unable to scale: CPU metrics unavailable for 10 minutes, stuck at 2/10 replicas",
				"summary":          "Horizontal Pod Autoscaler unable to function",
				"current_replicas": "2",
				"desired_replicas": "unable_to_calculate",
				"max_replicas":     "10",
				"target_cpu":       "70%",
				"metrics_error":    "cpu_metrics_unavailable",
				"duration":         "10m",
			},
			StartsAt: time.Now().Add(-10 * time.Minute),
		},
		ExpectedActions: []string{"scale_deployment", "restart_pod", "notify_only"},
		MinConfidence:   0.7,
		Description:     "HPA failures may require manual scaling or metrics system restart",
	},
	{
		Name: "ImagePullBackoffMultipleNodes",
		Alert: slm.Alert{
			Name:        "ImagePullBackOff",
			Status:      "firing",
			Severity:    "critical",
			Description: "Image registry timeout: 15 pods across 5 nodes unable to pull 'webapp:v2.1.3' for 20 minutes",
			Namespace:   "production",
			Resource:    "webapp-v2",
			Labels: map[string]string{
				"alertname":  "ImagePullBackOff",
				"severity":   "critical",
				"namespace":  "production",
				"deployment": "webapp",
				"image":      "webapp:v2.1.3",
				"registry":   "registry.company.com",
			},
			Annotations: map[string]string{
				"description":        "Image registry timeout: 15 pods across 5 nodes unable to pull 'webapp:v2.1.3' for 20 minutes",
				"summary":            "Critical image pull failures blocking deployment",
				"affected_pods":      "15",
				"affected_nodes":     "5",
				"duration":           "20m",
				"registry_error":     "timeout",
				"image_size":         "2.3GB",
				"rollback_available": "webapp:v2.1.2",
			},
			StartsAt: time.Now().Add(-20 * time.Minute),
		},
		ExpectedActions: []string{"notify_only"},
		MinConfidence:   0.8,
		Description:     "Registry issues require infrastructure or configuration fixes",
	},
	{
		Name: "ServiceMeshFailure",
		Alert: slm.Alert{
			Name:        "IstioSidecarFailing",
			Status:      "firing",
			Severity:    "critical",
			Description: "Istio sidecar in user-service pods crashing: mTLS certificate expired, 50% traffic failing",
			Namespace:   "production",
			Resource:    "user-service",
			Labels: map[string]string{
				"alertname":  "IstioSidecarFailing",
				"severity":   "critical",
				"namespace":  "production",
				"service":    "user-service",
				"deployment": "user-service",
				"mesh":       "istio",
			},
			Annotations: map[string]string{
				"description":   "Istio sidecar in user-service pods crashing: mTLS certificate expired, 50% traffic failing",
				"summary":       "Service mesh sidecar failures",
				"cert_status":   "expired",
				"failure_rate":  "50%",
				"affected_pods": "8/16",
				"tls_errors":    "certificate_expired",
				"mesh_version":  "1.18.2",
			},
			StartsAt: time.Now().Add(-5 * time.Minute),
		},
		ExpectedActions: []string{"restart_pod", "notify_only"},
		MinConfidence:   0.8,
		Description:     "Service mesh issues may require pod restart or certificate renewal",
	},
	{
		Name: "OOMKillerMultipleInstances",
		Alert: slm.Alert{
			Name:        "ContainerOOMKilled",
			Status:      "firing",
			Severity:    "critical",
			Description: "Analytics pods killed by OOM: 8 instances in 10 minutes, memory limit 2Gi insufficient",
			Namespace:   "analytics",
			Resource:    "analytics-worker",
			Labels: map[string]string{
				"alertname":  "ContainerOOMKilled",
				"severity":   "critical",
				"namespace":  "analytics",
				"deployment": "analytics-worker",
				"container":  "worker",
				"reason":     "OOMKilled",
			},
			Annotations: map[string]string{
				"description":   "Analytics pods killed by OOM: 8 instances in 10 minutes, memory limit 2Gi insufficient",
				"summary":       "Repeated OOM kills affecting service availability",
				"oom_count":     "8",
				"time_window":   "10m",
				"memory_limit":  "2Gi",
				"memory_usage":  "2.1Gi",
				"restart_count": "23",
				"availability":  "30%",
			},
			StartsAt: time.Now().Add(-10 * time.Minute),
		},
		ExpectedActions: []string{"increase_resources", "scale_deployment"},
		MinConfidence:   0.9,
		Description:     "OOM kills clearly indicate need for resource increase or scaling",
	},
	{
		Name: "CertificateExpiryImminent",
		Alert: slm.Alert{
			Name:        "CertificateExpiringSoon",
			Status:      "firing",
			Severity:    "warning",
			Description: "TLS certificate for api.company.com expires in 6 hours, used by 3 ingress routes",
			Namespace:   "ingress-nginx",
			Resource:    "api-tls-cert",
			Labels: map[string]string{
				"alertname":    "CertificateExpiringSoon",
				"severity":     "warning",
				"namespace":    "ingress-nginx",
				"secret":       "api-tls-cert",
				"domain":       "api.company.com",
				"cert_manager": "true",
			},
			Annotations: map[string]string{
				"description":      "TLS certificate for api.company.com expires in 6 hours, used by 3 ingress routes",
				"summary":          "TLS certificate expiring soon",
				"expires_in":       "6h",
				"domain":           "api.company.com",
				"issuer":           "letsencrypt-prod",
				"affected_ingress": "api-ingress, admin-ingress, mobile-api-ingress",
				"auto_renewal":     "enabled",
			},
			StartsAt: time.Now().Add(-1 * time.Hour),
		},
		ExpectedActions: []string{"notify_only"},
		MinConfidence:   0.9,
		Description:     "Certificate expiry requires monitoring and potential manual intervention",
	},
}

// Production Edge Case Test Groups for specialized testing
var (
	// ChaosEngineeringAlerts contains chaos testing scenarios
	ChaosEngineeringAlerts = []TestCase{
		{
			Name: "NetworkPartitionSimulation",
			Alert: slm.Alert{
				Name:        "NodeNetworkPartition",
				Status:      "firing",
				Severity:    "critical",
				Description: "Network partition detected: Node worker-05 isolated, 25 pods unreachable for 3 minutes",
				Namespace:   "kube-system",
				Resource:    "worker-05",
				Labels: map[string]string{
					"alertname":    "NodeNetworkPartition",
					"severity":     "critical",
					"node":         "worker-05",
					"failure_type": "network_partition",
					"chaos_test":   "true",
				},
				Annotations: map[string]string{
					"description":        "Network partition detected: Node worker-05 isolated, 25 pods unreachable for 3 minutes",
					"summary":            "Chaos engineering: Network partition simulation",
					"isolated_pods":      "25",
					"partition_duration": "3m",
					"network_segment":    "10.0.1.0/24",
					"recovery_action":    "manual_intervention_required",
				},
				StartsAt: time.Now().Add(-3 * time.Minute),
			},
			ExpectedActions: []string{"notify_only"},
			MinConfidence:   0.95,
			Description:     "Network partitions require infrastructure-level recovery",
		},
		{
			Name: "RandomPodTermination",
			Alert: slm.Alert{
				Name:        "ChaosMonkeyPodKill",
				Status:      "firing",
				Severity:    "warning",
				Description: "Chaos Monkey terminated 3 random pods in production namespace: order-service, user-cache, notification-worker",
				Namespace:   "production",
				Resource:    "chaos-monkey",
				Labels: map[string]string{
					"alertname":    "ChaosMonkeyPodKill",
					"severity":     "warning",
					"chaos_test":   "true",
					"experiment":   "pod_termination",
					"target_count": "3",
				},
				Annotations: map[string]string{
					"description":       "Chaos Monkey terminated 3 random pods in production namespace",
					"summary":           "Chaos engineering: Random pod termination",
					"killed_pods":       "order-service-789, user-cache-456, notification-worker-123",
					"experiment_id":     "chaos-2025-001",
					"expected_recovery": "automatic",
					"monitoring_window": "10m",
				},
				StartsAt: time.Now().Add(-1 * time.Minute),
			},
			ExpectedActions: []string{"notify_only", "restart_pod"},
			MinConfidence:   0.7,
			Description:     "Chaos engineering events should trigger monitoring but minimal interference",
		},
	}

	// SecurityComplianceAlerts contains security-focused test cases
	SecurityComplianceAlerts = []TestCase{
		{
			Name: "UnauthorizedAPIAccess",
			Alert: slm.Alert{
				Name:        "SuspiciousAPIActivity",
				Status:      "firing",
				Severity:    "critical",
				Description: "Unauthorized API access detected: 500 failed authentication attempts from IP 192.168.1.100 in 2 minutes",
				Namespace:   "kube-system",
				Resource:    "kubernetes-api",
				Labels: map[string]string{
					"alertname":      "SuspiciousAPIActivity",
					"severity":       "critical",
					"attack_type":    "brute_force",
					"source_ip":      "192.168.1.100",
					"security_event": "true",
				},
				Annotations: map[string]string{
					"description":         "Unauthorized API access detected: 500 failed authentication attempts from IP 192.168.1.100 in 2 minutes",
					"summary":             "Critical security incident: Potential API attack",
					"failed_attempts":     "500",
					"time_window":         "2m",
					"source_country":      "Unknown",
					"blocked_by_firewall": "false",
					"immediate_action":    "block_source_ip",
				},
				StartsAt: time.Now().Add(-2 * time.Minute),
			},
			ExpectedActions: []string{"notify_only"},
			MinConfidence:   0.95,
			Description:     "Security incidents require immediate human intervention and forensics",
		},
		{
			Name: "ComplianceViolationDetected",
			Alert: slm.Alert{
				Name:        "PodSecurityPolicyViolation",
				Status:      "firing",
				Severity:    "critical",
				Description: "Compliance violation: Pod finance-app-001 accessing restricted hostPath /etc/shadow, violating SOX policy",
				Namespace:   "finance",
				Resource:    "finance-app-001",
				Labels: map[string]string{
					"alertname":      "PodSecurityPolicyViolation",
					"severity":       "critical",
					"compliance":     "SOX",
					"violation_type": "unauthorized_host_access",
					"audit_required": "true",
				},
				Annotations: map[string]string{
					"description":     "Compliance violation: Pod finance-app-001 accessing restricted hostPath /etc/shadow",
					"summary":         "Critical compliance violation detected",
					"policy_violated": "SOX-2025-001",
					"host_path":       "/etc/shadow",
					"risk_level":      "high",
					"audit_trail":     "required",
					"remediation":     "immediate_quarantine",
				},
				StartsAt: time.Now().Add(-30 * time.Second),
			},
			ExpectedActions: []string{"restart_pod", "notify_only"},
			MinConfidence:   0.9,
			Description:     "Compliance violations require immediate quarantine and audit trail",
		},
	}

	// ResourceExhaustionAlerts contains resource stress scenarios
	ResourceExhaustionAlerts = []TestCase{
		{
			Name: "ClusterWideMemoryExhaustion",
			Alert: slm.Alert{
				Name:        "ClusterMemoryPressure",
				Status:      "firing",
				Severity:    "critical",
				Description: "Cluster-wide memory pressure: 8/10 nodes above 90% memory usage, 150 pods pending eviction",
				Namespace:   "kube-system",
				Resource:    "cluster-autoscaler",
				Labels: map[string]string{
					"alertname":      "ClusterMemoryPressure",
					"severity":       "critical",
					"scope":          "cluster_wide",
					"affected_nodes": "8",
					"total_nodes":    "10",
				},
				Annotations: map[string]string{
					"description":           "Cluster-wide memory pressure: 8/10 nodes above 90% memory usage, 150 pods pending eviction",
					"summary":               "Critical cluster resource exhaustion",
					"memory_pressure_nodes": "worker-01,worker-02,worker-03,worker-04,worker-05,worker-06,worker-07,worker-08",
					"pods_at_risk":          "150",
					"cluster_capacity":      "90%",
					"auto_scaling_status":   "at_max_capacity",
					"emergency_procedure":   "required",
				},
				StartsAt: time.Now().Add(-5 * time.Minute),
			},
			ExpectedActions: []string{"notify_only"},
			MinConfidence:   0.95,
			Description:     "Cluster-wide resource exhaustion requires infrastructure scaling decisions",
		},
		{
			Name: "FileDescriptorExhaustion",
			Alert: slm.Alert{
				Name:        "FileDescriptorLimit",
				Status:      "firing",
				Severity:    "critical",
				Description: "File descriptor exhaustion: Process logging-agent using 65530/65536 file descriptors, near system limit",
				Namespace:   "logging",
				Resource:    "logging-agent-daemonset",
				Labels: map[string]string{
					"alertname":  "FileDescriptorLimit",
					"severity":   "critical",
					"process":    "logging-agent",
					"limit_type": "file_descriptors",
					"node_wide":  "true",
				},
				Annotations: map[string]string{
					"description":    "File descriptor exhaustion: Process logging-agent using 65530/65536 file descriptors",
					"summary":        "Critical system resource limit reached",
					"current_usage":  "65530",
					"system_limit":   "65536",
					"utilization":    "99.99%",
					"impact":         "cannot_open_new_files",
					"affected_nodes": "all_worker_nodes",
				},
				StartsAt: time.Now().Add(-2 * time.Minute),
			},
			ExpectedActions: []string{"restart_pod", "notify_only"},
			MinConfidence:   0.85,
			Description:     "File descriptor exhaustion typically requires process restart",
		},
	}

	// CascadingFailureAlerts contains complex failure scenarios
	CascadingFailureAlerts = []TestCase{
		{
			Name: "DatabaseCascadingFailure",
			Alert: slm.Alert{
				Name:        "DatabaseClusterFailure",
				Status:      "firing",
				Severity:    "critical",
				Description: "Database cascade failure: Primary DB down → Read replicas overloaded → Connection pools exhausted → 15 services failing",
				Namespace:   "database",
				Resource:    "postgres-cluster",
				Labels: map[string]string{
					"alertname":         "DatabaseClusterFailure",
					"severity":          "critical",
					"failure_type":      "cascading",
					"primary_cause":     "database_primary_failure",
					"affected_services": "15",
				},
				Annotations: map[string]string{
					"description":         "Database cascade failure: Primary DB down → Read replicas overloaded → Connection pools exhausted → 15 services failing",
					"summary":             "Critical cascading database failure",
					"primary_status":      "down",
					"replica_status":      "overloaded",
					"connection_pools":    "exhausted",
					"failing_services":    "payment,user,order,inventory,notification,auth,analytics,reporting,audit,logging,metrics,backup,scheduler,webhook,cache",
					"estimated_downtime":  "30-60m",
					"recovery_complexity": "high",
				},
				StartsAt: time.Now().Add(-5 * time.Minute),
			},
			ExpectedActions: []string{"notify_only"},
			MinConfidence:   0.95,
			Description:     "Cascading failures require coordinated incident response",
		},
		{
			Name: "LoadBalancerFailoverChain",
			Alert: slm.Alert{
				Name:        "LoadBalancerCascade",
				Status:      "firing",
				Severity:    "critical",
				Description: "Load balancer cascade: Primary LB failed → Secondary LB overloaded → Health checks failing → Circuit breakers opening",
				Namespace:   "ingress-nginx",
				Resource:    "nginx-ingress-controller",
				Labels: map[string]string{
					"alertname":    "LoadBalancerCascade",
					"severity":     "critical",
					"failure_type": "infrastructure_cascade",
					"component":    "load_balancer",
				},
				Annotations: map[string]string{
					"description":        "Load balancer cascade: Primary LB failed → Secondary LB overloaded → Health checks failing → Circuit breakers opening",
					"summary":            "Critical load balancer cascade failure",
					"primary_lb":         "failed",
					"secondary_lb":       "overloaded_60000_connections",
					"health_checks":      "failing_80_percent",
					"circuit_breakers":   "open_12_services",
					"traffic_impact":     "90_percent_requests_failing",
					"failover_mechanism": "not_functioning",
				},
				StartsAt: time.Now().Add(-3 * time.Minute),
			},
			ExpectedActions: []string{"notify_only", "restart_pod"},
			MinConfidence:   0.9,
			Description:     "Infrastructure cascades require immediate traffic management",
		},
	}

	// MultiAlertCorrelationScenarios contains scenarios with multiple related alerts
	MultiAlertCorrelationScenarios = []TestCase{
		{
			Name: "StorageAndMemoryCorrelation",
			Alert: slm.Alert{
				Name:        "CorrelatedResourceIssue",
				Status:      "firing",
				Severity:    "critical",
				Description: "Correlated issue: Disk I/O saturation (95%) causing memory pressure (92%) and CPU steal time (30%) on worker-01",
				Namespace:   "kube-system",
				Resource:    "worker-01",
				Labels: map[string]string{
					"alertname":      "CorrelatedResourceIssue",
					"severity":       "critical",
					"correlation_id": "CORR-2025-001",
					"root_cause":     "disk_io_saturation",
					"affected_node":  "worker-01",
				},
				Annotations: map[string]string{
					"description":          "Correlated issue: Disk I/O saturation (95%) causing memory pressure (92%) and CPU steal time (30%)",
					"summary":              "Multiple correlated resource issues detected",
					"disk_io_usage":        "95%",
					"memory_usage":         "92%",
					"cpu_steal_time":       "30%",
					"correlation_strength": "high",
					"probable_root_cause":  "noisy_neighbor_workload",
					"affected_workloads":   "database,analytics,logging",
				},
				StartsAt: time.Now().Add(-4 * time.Minute),
			},
			ExpectedActions: []string{"notify_only", "restart_pod"},
			MinConfidence:   0.8,
			Description:     "Correlated resource issues require root cause analysis",
		},
	}
)

// PerformanceTestAlert creates an alert for performance testing
func PerformanceTestAlert(id int) slm.Alert {
	return slm.Alert{
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
			"test_id":   string(rune(id)),
			"test_type": "performance",
		},
		Annotations: map[string]string{
			"description": "Performance test alert for measuring response times",
			"summary":     "Performance test",
			"test_id":     string(rune(id)),
		},
		StartsAt: time.Now(),
	}
}

// ConcurrentTestAlert creates an alert for concurrent testing
func ConcurrentTestAlert(id int) slm.Alert {
	return slm.Alert{
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
			"test_id":   string(rune(id)),
			"test_type": "concurrent",
		},
		Annotations: map[string]string{
			"description": "Concurrent test alert for parallel processing validation",
			"summary":     "Concurrent test",
			"test_id":     string(rune(id)),
		},
		StartsAt: time.Now(),
	}
}

// MalformedAlert creates an intentionally malformed alert for error testing
func MalformedAlert() slm.Alert {
	return slm.Alert{
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

// Specialized Edge Case Test Functions for Production Scenarios

// ChaosEngineeringTestAlert generates chaos engineering scenarios
func ChaosEngineeringTestAlert(scenario string) slm.Alert {
	scenarios := map[string]slm.Alert{
		"cpu_stress": {
			Name:        "ChaosCPUStress",
			Status:      "firing",
			Severity:    "warning",
			Description: "Chaos engineering: CPU stress test injected 90% load on worker-03 for resilience testing",
			Namespace:   "chaos-engineering",
			Resource:    "cpu-stress-test",
			Labels: map[string]string{
				"alertname":    "ChaosCPUStress",
				"severity":     "warning",
				"chaos_test":   "true",
				"experiment":   "cpu_stress",
				"target_node":  "worker-03",
			},
			Annotations: map[string]string{
				"description": "Chaos engineering: CPU stress test injected 90% load on worker-03 for resilience testing",
				"summary":     "Chaos experiment: CPU stress test",
				"stress_level": "90%",
				"duration":     "10m",
				"experiment_id": fmt.Sprintf("chaos-cpu-%d", time.Now().Unix()),
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
				"alertname":    "ChaosMemoryLeak",
				"severity":     "critical",
				"chaos_test":   "true",
				"experiment":   "memory_leak",
			},
			Annotations: map[string]string{
				"description": "Chaos engineering: Memory leak simulation in analytics-worker consuming 8GB in 5 minutes",
				"summary":     "Chaos experiment: Memory leak simulation",
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
func SecurityIncidentAlert(incidentType string) slm.Alert {
	incidents := map[string]slm.Alert{
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
				"description": "Security incident: Attempted privilege escalation detected in pod web-app-789",
				"summary":     "Critical security incident: Privilege escalation attempt",
				"target_file": "/etc/passwd",
				"attack_vector": "container_escape",
				"user_id": "www-data",
				"attempted_uid": "0",
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
				"description": "Security alert: Unusual data access pattern detected - 10GB customer data accessed in 30 seconds",
				"summary":     "Potential data exfiltration attempt",
				"data_volume": "10GB",
				"access_time": "30s",
				"normal_rate": "100MB/min",
				"anomaly_factor": "200x",
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
func ResourceExhaustionAlert(resourceType string) slm.Alert {
	resources := map[string]slm.Alert{
		"inode_exhaustion": {
			Name:        "InodeExhaustion",
			Status:      "firing",
			Severity:    "critical",
			Description: "Inode exhaustion: Filesystem /var/lib/docker using 99.8% inodes (524280/524288), cannot create files",
			Namespace:   "kube-system",
			Resource:    "docker-storage",
			Labels: map[string]string{
				"alertname":   "InodeExhaustion",
				"severity":    "critical",
				"filesystem":  "/var/lib/docker",
				"resource":    "inodes",
			},
			Annotations: map[string]string{
				"description": "Inode exhaustion: Filesystem /var/lib/docker using 99.8% inodes",
				"summary":     "Critical filesystem inode exhaustion",
				"used_inodes": "524280",
				"total_inodes": "524288",
				"utilization": "99.8%",
				"impact": "cannot_create_new_files",
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
				"alertname":    "NetworkBandwidthSaturation",
				"severity":     "critical",
				"node":         "worker-02",
				"resource":     "network_bandwidth",
			},
			Annotations: map[string]string{
				"description": "Network bandwidth saturation: Node worker-02 using 9.8Gbps/10Gbps network capacity",
				"summary":     "Critical network bandwidth exhaustion",
				"current_usage": "9.8Gbps",
				"total_capacity": "10Gbps",
				"utilization": "98%",
				"packet_loss": "5%",
				"impact": "service_degradation",
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
func CascadingFailureAlert(scenario string) slm.Alert {
	scenarios := map[string]slm.Alert{
		"monitoring_cascade": {
			Name:        "MonitoringSystemCascade",
			Status:      "firing",
			Severity:    "critical",
			Description: "Monitoring cascade: Prometheus down → Alertmanager silent → HPA blind → Auto-scaling failed → Performance degraded",
			Namespace:   "monitoring",
			Resource:    "prometheus-server",
			Labels: map[string]string{
				"alertname":     "MonitoringSystemCascade",
				"severity":      "critical",
				"failure_type":  "monitoring_cascade",
				"impact_scope":  "cluster_wide",
			},
			Annotations: map[string]string{
				"description": "Monitoring cascade: Prometheus down → Alertmanager silent → HPA blind → Auto-scaling failed → Performance degraded",
				"summary":     "Critical monitoring system cascade failure",
				"prometheus_status": "down",
				"alertmanager_status": "silent",
				"hpa_status": "metrics_unavailable",
				"autoscaling_status": "failed",
				"affected_services": "all_monitored_services",
				"estimated_impact": "blind_operations",
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
				"alertname":     "StorageSystemCascade",
				"severity":      "critical",
				"failure_type":  "storage_cascade",
				"root_cause":    "san_failure",
			},
			Annotations: map[string]string{
				"description": "Storage cascade: SAN failure → PV unavailable → StatefulSets stuck → Database down → 20 services failing",
				"summary":     "Critical storage system cascade failure",
				"san_status": "failed",
				"pv_status": "unavailable",
				"statefulsets_status": "pending",
				"database_status": "down",
				"failing_services": "20",
				"estimated_recovery": "2-4_hours",
			},
			StartsAt: time.Now().Add(-10 * time.Minute),
		},
	}
	
	if alert, exists := scenarios[scenario]; exists {
		return alert
	}
	return scenarios["monitoring_cascade"] // Default
}

// GetAllEdgeCaseAlerts returns all edge case test scenarios combined
func GetAllEdgeCaseAlerts() []TestCase {
	var allEdgeCases []TestCase
	
	// Combine all edge case categories
	allEdgeCases = append(allEdgeCases, ChaosEngineeringAlerts...)
	allEdgeCases = append(allEdgeCases, SecurityComplianceAlerts...)
	allEdgeCases = append(allEdgeCases, ResourceExhaustionAlerts...)
	allEdgeCases = append(allEdgeCases, CascadingFailureAlerts...)
	allEdgeCases = append(allEdgeCases, MultiAlertCorrelationScenarios...)
	
	return allEdgeCases
}
