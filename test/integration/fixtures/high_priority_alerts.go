package fixtures

import (
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
)

// HighPriorityTestAlerts contains test cases for the 5 new high-priority actions
var HighPriorityTestAlerts = []TestCase{
	{
		Name: "DeploymentFailureRollback",
		Alert: types.Alert{
			Name:        "DeploymentFailure",
			Status:      "firing",
			Severity:    "critical",
			Description: "Deployment webapp failed rollout with 0 ready replicas",
			Namespace:   "production",
			Resource:    "webapp-deployment",
			Labels: map[string]string{
				"alertname":  "DeploymentFailure",
				"severity":   "critical",
				"namespace":  "production",
				"deployment": "webapp",
				"revision":   "5",
			},
			Annotations: map[string]string{
				"description":           "Deployment webapp failed rollout with 0 ready replicas",
				"summary":               "Critical deployment failure requiring rollback",
				"ready_replicas":        "0",
				"desired_replicas":      "3",
				"last_working_revision": "4",
			},
			StartsAt: time.Now().Add(-2 * time.Minute),
		},
		ExpectedActions: []string{"rollback_deployment", "notify_only"},
		MinConfidence:   0.8,
		Description:     "Failed deployments should trigger automatic rollback",
	},
	{
		Name: "StorageSpaceExhaustion",
		Alert: types.Alert{
			Name:        "PVCNearFull",
			Status:      "firing",
			Severity:    "warning",
			Description: "Persistent volume claim database-storage is 90% full",
			Namespace:   "database",
			Resource:    "database-storage",
			Labels: map[string]string{
				"alertname": "PVCNearFull",
				"severity":  "warning",
				"namespace": "database",
				"pvc":       "database-storage",
			},
			Annotations: map[string]string{
				"description":     "Persistent volume claim database-storage is 90% full",
				"summary":         "Storage near capacity",
				"usage_percent":   "90",
				"available_space": "1Gi",
				"total_space":     "10Gi",
			},
			StartsAt: time.Now().Add(-10 * time.Minute),
		},
		ExpectedActions: []string{"expand_pvc", "notify_only"},
		MinConfidence:   0.7,
		Description:     "Storage exhaustion should trigger PVC expansion",
	},
	{
		Name: "NodeMaintenanceRequired",
		Alert: types.Alert{
			Name:        "NodeMaintenanceRequired",
			Status:      "firing",
			Severity:    "warning",
			Description: "Node worker-02 requires maintenance and should be drained",
			Namespace:   "",
			Resource:    "worker-02",
			Labels: map[string]string{
				"alertname": "NodeMaintenanceRequired",
				"severity":  "warning",
				"node":      "worker-02",
				"reason":    "kernel_update",
			},
			Annotations: map[string]string{
				"description":        "Node worker-02 requires maintenance and should be drained",
				"summary":            "Node maintenance scheduled",
				"maintenance_reason": "kernel_update",
				"maintenance_window": "2h",
			},
			StartsAt: time.Now().Add(-5 * time.Minute),
		},
		ExpectedActions: []string{"drain_node", "notify_only"},
		MinConfidence:   0.8,
		Description:     "Node maintenance should trigger safe pod eviction",
	},
	{
		Name: "SecurityThreatDetected",
		Alert: types.Alert{
			Name:        "SecurityThreatDetected",
			Status:      "firing",
			Severity:    "critical",
			Description: "Suspicious activity detected in pod web-app-456",
			Namespace:   "production",
			Resource:    "web-app-456",
			Labels: map[string]string{
				"alertname":   "SecurityThreatDetected",
				"severity":    "critical",
				"namespace":   "production",
				"pod":         "web-app-456",
				"threat_type": "malware",
			},
			Annotations: map[string]string{
				"description":     "Suspicious activity detected in pod web-app-456",
				"summary":         "Security threat requiring immediate isolation",
				"threat_type":     "malware",
				"confidence":      "high",
				"action_required": "quarantine",
			},
			StartsAt: time.Now().Add(-1 * time.Minute),
		},
		ExpectedActions: []string{"quarantine_pod", "notify_only"},
		MinConfidence:   0.9,
		Description:     "Security threats should trigger pod quarantine",
	},
	{
		Name: "ComplexTroubleshooting",
		Alert: types.Alert{
			Name:        "ComplexServiceFailure",
			Status:      "firing",
			Severity:    "critical",
			Description: "Service payment-api experiencing intermittent failures",
			Namespace:   "production",
			Resource:    "payment-api-789",
			Labels: map[string]string{
				"alertname": "ComplexServiceFailure",
				"severity":  "critical",
				"namespace": "production",
				"service":   "payment-api",
				"pod":       "payment-api-789",
			},
			Annotations: map[string]string{
				"description":          "Service payment-api experiencing intermittent failures",
				"summary":              "Complex failure requiring diagnostic analysis",
				"error_rate":           "25%",
				"symptoms":             "timeout,connection_refused",
				"investigation_needed": "true",
			},
			StartsAt: time.Now().Add(-15 * time.Minute),
		},
		ExpectedActions: []string{"collect_diagnostics", "notify_only", "restart_pod"},
		MinConfidence:   0.6,
		Description:     "Complex failures should trigger diagnostic collection",
	},
}
