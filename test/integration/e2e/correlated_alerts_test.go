//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/jordigilh/kubernaut/test/integration/shared"
)

func TestCorrelatedAlertsIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Correlated Alerts Integration Suite")
}

var _ = Describe("Correlated Alerts and Root Cause Analysis", Ordered, func() {
	var (
		logger       *logrus.Logger
		stateManager *shared.ComprehensiveStateManager
	)

	BeforeAll(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		// Use comprehensive state manager with database isolation
		patterns := &shared.TestIsolationPatterns{}
		stateManager = patterns.DatabaseTransactionIsolatedSuite("Correlated Alerts and Root Cause Analysis")

		testConfig := shared.LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests disabled")
		}
	})

	AfterAll(func() {
		// Comprehensive cleanup
		err := stateManager.CleanupAllState()
		Expect(err).ToNot(HaveOccurred())
	})

	BeforeEach(func() {
		logger.Debug("Starting correlated alerts test with isolated state")
	})

	AfterEach(func() {
		logger.Debug("Correlated alerts test completed - state automatically isolated")
	})

	getRepository := func() actionhistory.Repository {
		// Get isolated database repository from state manager
		dbHelper := stateManager.GetDatabaseHelper()
		return dbHelper.GetRepository()
	}

	createSLMClient := func() llm.Client {
		// Use fake SLM client instead of real client to eliminate external dependencies
		return shared.NewFakeSLMClient()
	}

	simulateAlertHistory := func(alertName, resource, action string, timestamp time.Time, effectiveness float64) {
		actionRecord := &actionhistory.ActionRecord{
			ResourceReference: actionhistory.ResourceReference{
				Namespace: "production",
				Kind:      "Deployment",
				Name:      resource,
			},
			ActionID:  fmt.Sprintf("sim-%s-%d", action, timestamp.Unix()),
			Timestamp: timestamp,
			Alert: actionhistory.AlertContext{
				Name:        alertName,
				Severity:    "warning",
				Labels:      map[string]string{"simulation": "true"},
				Annotations: map[string]string{"simulation": "true"},
				FiringTime:  timestamp,
			},
			ModelUsed:           "fake-test-model",
			Confidence:          0.8,
			Reasoning:           func(s string) *string { return &s }("Simulated alert history"),
			ActionType:          action,
			Parameters:          map[string]interface{}{"simulated": true},
			ResourceStateBefore: map[string]interface{}{"status": "before"},
			ResourceStateAfter:  map[string]interface{}{"status": "after"},
		}

		trace, err := getRepository().StoreAction(context.Background(), actionRecord)
		Expect(err).ToNot(HaveOccurred())

		trace.EffectivenessScore = &effectiveness
		trace.ExecutionStatus = "completed"
		err = getRepository().UpdateActionTrace(context.Background(), trace)
		Expect(err).ToNot(HaveOccurred())
	}

	Context("Infrastructure-Level Correlation Scenarios", func() {
		It("should prioritize node-level issues over pod-level symptoms", func() {
			client := createSLMClient()

			// Scenario: Node network issues causing pod crashes
			// First: Process the symptom alert (pod crash)
			podCrashAlert := types.Alert{
				Name:        "PodCrashLooping",
				Status:      "firing",
				Severity:    "critical",
				Description: "Pod web-app-pod-123 is crash looping",
				Namespace:   "production",
				Resource:    "web-app-pod-123",
				Labels: map[string]string{
					"alertname":     "PodCrashLooping",
					"pod":           "web-app-pod-123",
					"node":          "worker-node-01",
					"restart_count": "15",
				},
				Annotations: map[string]string{
					"description": "Pod has restarted 15 times in the last 10 minutes",
					"node":        "worker-node-01",
					"exit_code":   "1",
				},
			}

			podRecommendation, err := client.AnalyzeAlert(context.Background(), podCrashAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(podRecommendation).ToNot(BeNil())

			// Then: Process the root cause alert (node network issues)
			nodeNetworkAlert := types.Alert{
				Name:        "NodeNetworkIssue",
				Status:      "firing",
				Severity:    "critical",
				Description: "Node worker-node-01 kubelet failing heartbeat - network connectivity issues",
				Namespace:   "",
				Resource:    "worker-node-01",
				Labels: map[string]string{
					"alertname":         "NodeNetworkIssue",
					"node":              "worker-node-01",
					"kubelet_status":    "down",
					"network_reachable": "false",
					"last_heartbeat":    "5m",
				},
				Annotations: map[string]string{
					"description":     "Kubelet heartbeat failing, network connectivity issues detected",
					"affected_pods":   "15",
					"network_latency": "timeout",
					"remediation":     "node_level_action_required",
				},
			}

			nodeRecommendation, err := client.AnalyzeAlert(context.Background(), nodeNetworkAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(nodeRecommendation).ToNot(BeNil())

			// Validate root cause prioritization
			// Node-level issues should trigger node-focused actions
			Expect(nodeRecommendation.Action).To(BeElementOf([]string{
				"drain_node",
				"notify_only",
				"collect_diagnostics",
			}), "Node network issues should trigger node-level remediation")

			// Pod-level symptoms might trigger pod actions initially
			// Confidence should be reasonable but could be high if model is confident about action
			if podRecommendation.Action == "restart_pod" {
				Expect(podRecommendation.Confidence).To(BeNumerically(">=", 0.5),
					"Pod restart should maintain reasonable confidence even with potential node issues")
			}

			// Node issue should have higher urgency/confidence for infrastructure action
			if nodeRecommendation.Action == "drain_node" {
				Expect(nodeRecommendation.Confidence).To(BeNumerically(">=", 0.7),
					"Node drain should have reasonable confidence for network issues")
			}

			logger.WithFields(logrus.Fields{
				"scenario":        "node_network_pod_crash",
				"pod_action":      podRecommendation.Action,
				"pod_confidence":  podRecommendation.Confidence,
				"node_action":     nodeRecommendation.Action,
				"node_confidence": nodeRecommendation.Confidence,
			}).Info("Infrastructure correlation test completed")
		})

		It("should handle storage-related cascading failures appropriately", func() {
			client := createSLMClient()

			// Scenario: Storage full -> Database pod issues -> Application failures

			// Root cause: Storage space exhaustion
			storageAlert := types.Alert{
				Name:        "PVCNearFull",
				Status:      "firing",
				Severity:    "critical",
				Description: "Database storage PVC is 98% full",
				Namespace:   "database",
				Resource:    "postgres-storage-pvc",
				Labels: map[string]string{
					"alertname":  "PVCNearFull",
					"pvc":        "postgres-storage-pvc",
					"usage":      "98%",
					"mount_path": "/var/lib/postgresql/data",
				},
				Annotations: map[string]string{
					"description":  "PostgreSQL data volume is critically full",
					"growth_rate":  "5%_per_hour",
					"time_to_full": "24m",
				},
			}

			storageRecommendation, err := client.AnalyzeAlert(context.Background(), storageAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(storageRecommendation).ToNot(BeNil())

			// Symptom 1: Database connection failures
			dbConnectionAlert := types.Alert{
				Name:        "DatabaseConnectionFailure",
				Status:      "firing",
				Severity:    "critical",
				Description: "Applications unable to connect to PostgreSQL database",
				Namespace:   "database",
				Resource:    "postgres-primary",
				Labels: map[string]string{
					"alertname":   "DatabaseConnectionFailure",
					"service":     "postgres-primary",
					"error_type":  "connection_refused",
					"storage_pvc": "postgres-storage-pvc",
				},
				Annotations: map[string]string{
					"description": "Database refusing connections, logs indicate disk full errors",
					"error_rate":  "100%",
					"root_cause":  "storage_exhaustion",
				},
			}

			dbRecommendation, err := client.AnalyzeAlert(context.Background(), dbConnectionAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(dbRecommendation).ToNot(BeNil())

			// Symptom 2: Application service degradation
			appDegradationAlert := types.Alert{
				Name:        "ServiceDegraded",
				Status:      "firing",
				Severity:    "critical",
				Description: "User service experiencing high error rate due to database issues",
				Namespace:   "production",
				Resource:    "user-service",
				Labels: map[string]string{
					"alertname":  "ServiceDegraded",
					"service":    "user-service",
					"error_rate": "85%",
					"dependency": "postgres-primary",
				},
				Annotations: map[string]string{
					"description":    "Service errors caused by database connectivity failures",
					"upstream_error": "database_unavailable",
					"impact":         "user_facing",
				},
			}

			appRecommendation, err := client.AnalyzeAlert(context.Background(), appDegradationAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(appRecommendation).ToNot(BeNil())

			// Validate cascading failure handling
			// Storage issue should get storage-focused remedy
			Expect(storageRecommendation.Action).To(BeElementOf([]string{
				"expand_pvc",
				"notify_only",
			}), "Storage exhaustion should trigger storage expansion")

			// Database and application issues should avoid actions that don't address root cause
			// They should either escalate or address storage if they understand the correlation
			Expect(dbRecommendation.Action).To(BeElementOf([]string{
				"notify_only",
				"collect_diagnostics",
				"expand_pvc", // If it understands the storage connection
			}), "Database issues caused by storage should not restart DB uselessly")

			Expect(appRecommendation.Action).To(BeElementOf([]string{
				"notify_only",
				"collect_diagnostics",
				"expand_pvc", // If it understands the root cause
			}), "App issues caused by storage should focus on root cause")

			logger.WithFields(logrus.Fields{
				"scenario":           "storage_cascade_failure",
				"storage_action":     storageRecommendation.Action,
				"db_action":          dbRecommendation.Action,
				"app_action":         appRecommendation.Action,
				"storage_confidence": storageRecommendation.Confidence,
				"db_confidence":      dbRecommendation.Confidence,
				"app_confidence":     appRecommendation.Confidence,
			}).Info("Storage cascading failure test completed")
		})
	})

	Context("Application-Level Correlation Scenarios", func() {
		It("should handle memory leak causing multiple related alerts", func() {
			client := createSLMClient()

			// Scenario: Memory leak in application causing multiple symptoms

			// Early symptom: High memory usage
			memoryAlert := types.Alert{
				Name:        "HighMemoryUsage",
				Status:      "firing",
				Severity:    "warning",
				Description: "Application memory usage trending upward",
				Namespace:   "production",
				Resource:    "api-service",
				Labels: map[string]string{
					"alertname":    "HighMemoryUsage",
					"deployment":   "api-service",
					"memory_usage": "85%",
					"trend":        "increasing",
				},
				Annotations: map[string]string{
					"description": "Memory usage has increased 40% in last hour",
					"pattern":     "continuous_growth",
					"baseline":    "45%",
				},
			}

			memoryRecommendation, err := client.AnalyzeAlert(context.Background(), memoryAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(memoryRecommendation).ToNot(BeNil())

			// Escalated symptom: OOM kills starting
			oomAlert := types.Alert{
				Name:        "ContainerOOMKilled",
				Status:      "firing",
				Severity:    "critical",
				Description: "Container killed due to out of memory",
				Namespace:   "production",
				Resource:    "api-service",
				Labels: map[string]string{
					"alertname":  "ContainerOOMKilled",
					"deployment": "api-service",
					"container":  "api-service-container",
					"oom_count":  "3",
				},
				Annotations: map[string]string{
					"description":  "Container has been OOM killed 3 times in 10 minutes",
					"memory_limit": "2Gi",
					"last_memory":  "2.1Gi",
				},
			}

			oomRecommendation, err := client.AnalyzeAlert(context.Background(), oomAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(oomRecommendation).ToNot(BeNil())

			// Performance degradation symptom
			performanceAlert := types.Alert{
				Name:        "HighResponseTime",
				Status:      "firing",
				Severity:    "warning",
				Description: "API response time significantly elevated",
				Namespace:   "production",
				Resource:    "api-service",
				Labels: map[string]string{
					"alertname":    "HighResponseTime",
					"service":      "api-service",
					"p95_latency":  "2500ms",
					"baseline_p95": "150ms",
				},
				Annotations: map[string]string{
					"description":       "Response time degraded likely due to memory pressure",
					"concurrent_alerts": "HighMemoryUsage,ContainerOOMKilled",
				},
			}

			performanceRecommendation, err := client.AnalyzeAlert(context.Background(), performanceAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(performanceRecommendation).ToNot(BeNil())

			// Validate memory leak correlation handling
			// Early memory alert might trigger scaling or resource increase
			Expect(memoryRecommendation.Action).To(BeElementOf([]string{
				"increase_resources",
				"scale_deployment",
				"restart_pod",
				"notify_only",
			}), "High memory might trigger various resource actions")

			// OOM kills should definitely trigger resource actions (NOT scaling)
			Expect(oomRecommendation.Action).To(BeElementOf([]string{
				"increase_resources", // PRIMARY: Fix memory limits
				"restart_pod",        // SECONDARY: Clear potential leaks
				"notify_only",        // FALLBACK: Human intervention
			}), "OOM kills indicate resource limits issue, NOT capacity problem")

			// OOM kills should NEVER trigger scaling (adds more failing containers)
			Expect(oomRecommendation.Action).ToNot(Equal("scale_deployment"),
				"Scaling deployment will not fix OOM kills and creates more failing pods")

			// Performance degradation in context of memory issues should address root cause
			Expect(performanceRecommendation.Action).To(BeElementOf([]string{
				"increase_resources",
				"restart_pod",
				"notify_only",
				"collect_diagnostics",
			}), "Performance issues with memory correlation should address memory")

			// At least one should suggest increasing resources for memory leak
			actions := []string{memoryRecommendation.Action, oomRecommendation.Action, performanceRecommendation.Action}
			Expect(actions).To(ContainElement("increase_resources"),
				"At least one alert should suggest increasing resources for memory leak")

			logger.WithFields(logrus.Fields{
				"scenario":               "memory_leak_correlation",
				"memory_action":          memoryRecommendation.Action,
				"oom_action":             oomRecommendation.Action,
				"performance_action":     performanceRecommendation.Action,
				"memory_confidence":      memoryRecommendation.Confidence,
				"oom_confidence":         oomRecommendation.Confidence,
				"performance_confidence": performanceRecommendation.Confidence,
			}).Info("Memory leak correlation test completed")
		})

		It("should handle deployment-related cascading failures", func() {
			client := createSLMClient()

			// Simulate previous successful rollbacks for this service
			simulateAlertHistory("DeploymentFailure", "frontend-service", "rollback_deployment",
				time.Now().Add(-2*time.Hour), 0.9)
			simulateAlertHistory("DeploymentFailure", "frontend-service", "rollback_deployment",
				time.Now().Add(-6*time.Hour), 0.85)

			// Scenario: Bad deployment causing multiple cascading issues

			// Root cause: Deployment failure
			deploymentAlert := types.Alert{
				Name:        "DeploymentFailure",
				Status:      "firing",
				Severity:    "critical",
				Description: "Frontend service deployment failed - 0 ready replicas",
				Namespace:   "production",
				Resource:    "frontend-service",
				Labels: map[string]string{
					"alertname":         "DeploymentFailure",
					"deployment":        "frontend-service",
					"ready_replicas":    "0",
					"desired_replicas":  "5",
					"revision":          "47",
					"previous_revision": "46",
				},
				Annotations: map[string]string{
					"description":  "New deployment revision 47 failing to start",
					"image_tag":    "v2.1.3",
					"previous_tag": "v2.1.2",
				},
			}

			deploymentRecommendation, err := client.AnalyzeAlert(context.Background(), deploymentAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(deploymentRecommendation).ToNot(BeNil())

			// Symptom 1: Service unavailable
			serviceAlert := types.Alert{
				Name:        "ServiceUnavailable",
				Status:      "firing",
				Severity:    "critical",
				Description: "Frontend service returning 503 errors",
				Namespace:   "production",
				Resource:    "frontend-service",
				Labels: map[string]string{
					"alertname":   "ServiceUnavailable",
					"service":     "frontend-service",
					"status_code": "503",
					"error_rate":  "100%",
					"deployment":  "frontend-service",
				},
				Annotations: map[string]string{
					"description":   "Service unavailable due to deployment failure",
					"related_alert": "DeploymentFailure",
				},
			}

			serviceRecommendation, err := client.AnalyzeAlert(context.Background(), serviceAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(serviceRecommendation).ToNot(BeNil())

			// Symptom 2: Load balancer errors
			lbAlert := types.Alert{
				Name:        "LoadBalancerNoBackends",
				Status:      "firing",
				Severity:    "critical",
				Description: "Load balancer has no healthy backends for frontend service",
				Namespace:   "production",
				Resource:    "frontend-lb",
				Labels: map[string]string{
					"alertname":        "LoadBalancerNoBackends",
					"service":          "frontend-service",
					"healthy_backends": "0",
					"total_backends":   "5",
				},
				Annotations: map[string]string{
					"description":      "No healthy backends due to deployment failure",
					"upstream_service": "frontend-service",
				},
			}

			lbRecommendation, err := client.AnalyzeAlert(context.Background(), lbAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(lbRecommendation).ToNot(BeNil())

			// Validate deployment failure correlation
			// Deployment failure should trigger rollback (given successful history)
			Expect(deploymentRecommendation.Action).To(BeElementOf([]string{
				"rollback_deployment",
				"notify_only",
				"collect_diagnostics",
			}), "Deployment failure should consider rollback given successful history")

			// Service and LB alerts should either rollback or escalate
			// (they shouldn't try to restart/scale when deployment is the issue)
			Expect(serviceRecommendation.Action).To(BeElementOf([]string{
				"rollback_deployment",
				"notify_only",
				"collect_diagnostics",
			}), "Service unavailable due to deployment should address deployment")

			Expect(lbRecommendation.Action).To(BeElementOf([]string{
				"rollback_deployment",
				"notify_only",
				"collect_diagnostics",
			}), "LB no backends due to deployment should address deployment")

			// Should not try scaling/restarting when deployment is fundamentally broken
			Expect(serviceRecommendation.Action).ToNot(BeElementOf([]string{
				"scale_deployment",
				"restart_pod",
			}), "Should not scale/restart when deployment is broken")

			logger.WithFields(logrus.Fields{
				"scenario":              "deployment_cascade_failure",
				"deployment_action":     deploymentRecommendation.Action,
				"service_action":        serviceRecommendation.Action,
				"lb_action":             lbRecommendation.Action,
				"deployment_confidence": deploymentRecommendation.Confidence,
				"service_confidence":    serviceRecommendation.Confidence,
				"lb_confidence":         lbRecommendation.Confidence,
			}).Info("Deployment cascading failure test completed")
		})
	})

	Context("Cross-Namespace Correlation Scenarios", func() {
		It("should handle database issues affecting multiple application namespaces", func() {
			client := createSLMClient()

			// Scenario: Database in 'database' namespace affecting apps in multiple namespaces

			// Root cause: Database performance degradation
			dbAlert := types.Alert{
				Name:        "DatabaseSlowQueries",
				Status:      "firing",
				Severity:    "warning",
				Description: "PostgreSQL queries taking longer than normal",
				Namespace:   "database",
				Resource:    "postgres-primary",
				Labels: map[string]string{
					"alertname":      "DatabaseSlowQueries",
					"database":       "postgres-primary",
					"avg_query_time": "2500ms",
					"baseline":       "150ms",
				},
				Annotations: map[string]string{
					"description":    "Database performance degraded, affecting all clients",
					"slow_query_pct": "85%",
					"affected_apps":  "user-service,order-service,payment-service",
				},
			}

			dbRecommendation, err := client.AnalyzeAlert(context.Background(), dbAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(dbRecommendation).ToNot(BeNil())

			// Symptom 1: User service issues in production namespace
			userServiceAlert := types.Alert{
				Name:        "HighResponseTime",
				Status:      "firing",
				Severity:    "warning",
				Description: "User service API response time elevated",
				Namespace:   "production",
				Resource:    "user-service",
				Labels: map[string]string{
					"alertname":     "HighResponseTime",
					"service":       "user-service",
					"response_time": "2800ms",
					"baseline":      "200ms",
					"database_dep":  "postgres-primary",
				},
				Annotations: map[string]string{
					"description":   "Response time degraded, correlates with database issues",
					"db_connection": "postgres-primary.database.svc.cluster.local",
				},
			}

			userServiceRecommendation, err := client.AnalyzeAlert(context.Background(), userServiceAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(userServiceRecommendation).ToNot(BeNil())

			// Symptom 2: Order service issues in ecommerce namespace
			orderServiceAlert := types.Alert{
				Name:        "ServiceTimeout",
				Status:      "firing",
				Severity:    "critical",
				Description: "Order service experiencing timeouts",
				Namespace:   "ecommerce",
				Resource:    "order-service",
				Labels: map[string]string{
					"alertname":    "ServiceTimeout",
					"service":      "order-service",
					"timeout_rate": "45%",
					"database_dep": "postgres-primary",
				},
				Annotations: map[string]string{
					"description":     "Service timeouts likely due to database performance",
					"db_connection":   "postgres-primary.database.svc.cluster.local",
					"business_impact": "order_processing_delayed",
				},
			}

			orderServiceRecommendation, err := client.AnalyzeAlert(context.Background(), orderServiceAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(orderServiceRecommendation).ToNot(BeNil())

			// Validate cross-namespace correlation
			// Database alert should focus on database remediation
			Expect(dbRecommendation.Action).To(BeElementOf([]string{
				"restart_pod",
				"increase_resources",
				"scale_deployment",
				"notify_only",
				"collect_diagnostics",
			}), "Database performance should trigger database-focused actions")

			// Application services should either escalate or address database if they understand correlation
			Expect(userServiceRecommendation.Action).To(BeElementOf([]string{
				"notify_only",
				"collect_diagnostics",
				"restart_pod",        // Might help if they're holding stale connections
				"increase_resources", // If they understand DB is the bottleneck
			}), "User service affected by DB should avoid irrelevant scaling")

			Expect(orderServiceRecommendation.Action).To(BeElementOf([]string{
				"notify_only",
				"collect_diagnostics",
				"restart_pod",        // Might help with connection pooling
				"increase_resources", // If they understand DB is bottleneck
			}), "Order service affected by DB should avoid irrelevant actions")

			// At least one should escalate to handle the cross-namespace correlation properly
			actions := []string{dbRecommendation.Action, userServiceRecommendation.Action, orderServiceRecommendation.Action}
			Expect(actions).To(ContainElement(BeElementOf([]string{"notify_only", "collect_diagnostics"})),
				"At least one alert should escalate for cross-namespace correlation analysis")

			logger.WithFields(logrus.Fields{
				"scenario":                 "cross_namespace_db_correlation",
				"db_action":                dbRecommendation.Action,
				"user_service_action":      userServiceRecommendation.Action,
				"order_service_action":     orderServiceRecommendation.Action,
				"db_confidence":            dbRecommendation.Confidence,
				"user_service_confidence":  userServiceRecommendation.Confidence,
				"order_service_confidence": orderServiceRecommendation.Confidence,
			}).Info("Cross-namespace correlation test completed")
		})
	})

	Context("Temporal Correlation Scenarios", func() {
		It("should handle alerts triggered in rapid succession with causal relationships", func() {
			client := createSLMClient()

			// Scenario: Network partition causing sequential failures

			// T+0: Network connectivity alert
			networkAlert := types.Alert{
				Name:        "NetworkConnectivityIssue",
				Status:      "firing",
				Severity:    "critical",
				Description: "Network partition detected between nodes",
				Namespace:   "",
				Resource:    "cluster-network",
				Labels: map[string]string{
					"alertname":      "NetworkConnectivityIssue",
					"affected_nodes": "worker-node-01,worker-node-02",
					"partition_type": "inter_node",
				},
				Annotations: map[string]string{
					"description":       "Network partition affecting inter-node communication",
					"detection_time":    time.Now().Format(time.RFC3339),
					"affected_services": "etcd,kubelet,kube-proxy",
				},
			}

			networkRecommendation, err := client.AnalyzeAlert(context.Background(), networkAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(networkRecommendation).ToNot(BeNil())

			// T+30s: Pod scheduling failures due to network issues
			schedulingAlert := types.Alert{
				Name:        "PodSchedulingFailure",
				Status:      "firing",
				Severity:    "critical",
				Description: "Pods failing to schedule due to node communication issues",
				Namespace:   "production",
				Resource:    "api-deployment",
				Labels: map[string]string{
					"alertname":         "PodSchedulingFailure",
					"deployment":        "api-deployment",
					"pending_pods":      "8",
					"scheduling_reason": "NodeNetworkUnavailable",
				},
				Annotations: map[string]string{
					"description":      "Pod scheduling failing due to network partition",
					"related_incident": "NetworkConnectivityIssue",
					"time_offset":      "30s",
				},
			}

			schedulingRecommendation, err := client.AnalyzeAlert(context.Background(), schedulingAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(schedulingRecommendation).ToNot(BeNil())

			// T+60s: Service discovery failures
			serviceDiscoveryAlert := types.Alert{
				Name:        "ServiceDiscoveryFailure",
				Status:      "firing",
				Severity:    "critical",
				Description: "Services unable to discover backends due to network issues",
				Namespace:   "production",
				Resource:    "service-mesh",
				Labels: map[string]string{
					"alertname":          "ServiceDiscoveryFailure",
					"affected_services":  "api-service,web-service",
					"discovery_failures": "100%",
				},
				Annotations: map[string]string{
					"description": "Service mesh unable to maintain service registry",
					"root_cause":  "network_partition",
					"time_offset": "60s",
				},
			}

			serviceDiscoveryRecommendation, err := client.AnalyzeAlert(context.Background(), serviceDiscoveryAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(serviceDiscoveryRecommendation).ToNot(BeNil())

			// Validate temporal correlation handling
			// Network issue should trigger network-focused remediation
			Expect(networkRecommendation.Action).To(BeElementOf([]string{
				"notify_only",
				"collect_diagnostics",
				"drain_node", // If specific nodes are affected
			}), "Network partition should trigger infrastructure-level response")

			// Subsequent alerts should avoid actions that don't address root cause
			Expect(schedulingRecommendation.Action).To(BeElementOf([]string{
				"notify_only",
				"collect_diagnostics",
			}), "Scheduling failures due to network should escalate, not scale")

			Expect(serviceDiscoveryRecommendation.Action).To(BeElementOf([]string{
				"notify_only",
				"collect_diagnostics",
			}), "Service discovery failures due to network should escalate")

			// Should not try scaling/restarting when network is the fundamental issue
			Expect(schedulingRecommendation.Action).ToNot(BeElementOf([]string{
				"scale_deployment",
				"restart_pod",
			}), "Should not scale when network partition is the issue")

			// Network issue should have highest confidence since it's the root cause
			if networkRecommendation.Action == "notify_only" {
				Expect(networkRecommendation.Confidence).To(BeNumerically(">=", 0.7),
					"Network partition should have high confidence for escalation")
			}

			logger.WithFields(logrus.Fields{
				"scenario":                     "temporal_network_correlation",
				"network_action":               networkRecommendation.Action,
				"scheduling_action":            schedulingRecommendation.Action,
				"service_discovery_action":     serviceDiscoveryRecommendation.Action,
				"network_confidence":           networkRecommendation.Confidence,
				"scheduling_confidence":        schedulingRecommendation.Confidence,
				"service_discovery_confidence": serviceDiscoveryRecommendation.Confidence,
			}).Info("Temporal correlation test completed")
		})
	})
})
