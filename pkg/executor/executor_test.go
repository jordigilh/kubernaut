package executor

import (
	"context"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/actionhistory"
	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// SimpleMockRepository for testing
type SimpleMockRepository struct{}

func (m *SimpleMockRepository) EnsureResourceReference(ctx context.Context, ref actionhistory.ResourceReference) (int64, error) {
	return 1, nil
}

func (m *SimpleMockRepository) GetResourceReference(ctx context.Context, namespace, kind, name string) (*actionhistory.ResourceReference, error) {
	return nil, nil
}

func (m *SimpleMockRepository) EnsureActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	return nil, nil
}

func (m *SimpleMockRepository) GetActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	return nil, nil
}

func (m *SimpleMockRepository) UpdateActionHistory(ctx context.Context, history *actionhistory.ActionHistory) error {
	return nil
}

func (m *SimpleMockRepository) StoreAction(ctx context.Context, action *actionhistory.ActionRecord) (*actionhistory.ResourceActionTrace, error) {
	return nil, nil
}

func (m *SimpleMockRepository) GetActionTraces(ctx context.Context, query actionhistory.ActionQuery) ([]actionhistory.ResourceActionTrace, error) {
	return nil, nil
}

func (m *SimpleMockRepository) GetActionTrace(ctx context.Context, actionID string) (*actionhistory.ResourceActionTrace, error) {
	return nil, nil
}

func (m *SimpleMockRepository) UpdateActionTrace(ctx context.Context, trace *actionhistory.ResourceActionTrace) error {
	return nil
}

func (m *SimpleMockRepository) GetPendingEffectivenessAssessments(ctx context.Context) ([]*actionhistory.ResourceActionTrace, error) {
	return nil, nil
}

func (m *SimpleMockRepository) GetOscillationPatterns(ctx context.Context, patternType string) ([]actionhistory.OscillationPattern, error) {
	return nil, nil
}

func (m *SimpleMockRepository) StoreOscillationDetection(ctx context.Context, detection *actionhistory.OscillationDetection) error {
	return nil
}

func (m *SimpleMockRepository) GetOscillationDetections(ctx context.Context, resourceID int64, resolved *bool) ([]actionhistory.OscillationDetection, error) {
	return nil, nil
}

func (m *SimpleMockRepository) ApplyRetention(ctx context.Context, actionHistoryID int64) error {
	return nil
}

func (m *SimpleMockRepository) GetActionHistorySummaries(ctx context.Context, since time.Duration) ([]actionhistory.ActionHistorySummary, error) {
	return nil, nil
}

// FakeK8sClient implements our k8s.Client interface using the Kubernetes fake client
type FakeK8sClient struct {
	clientset *fake.Clientset
	log       *logrus.Logger
}

func NewFakeK8sClient(objects ...runtime.Object) *FakeK8sClient {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	return &FakeK8sClient{
		clientset: fake.NewSimpleClientset(objects...),
		log:       logger,
	}
}

// BasicClient methods
func (f *FakeK8sClient) GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	return f.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (f *FakeK8sClient) DeletePod(ctx context.Context, namespace, name string) error {
	return f.clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (f *FakeK8sClient) ListPodsWithLabel(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error) {
	return f.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
}

func (f *FakeK8sClient) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	return f.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (f *FakeK8sClient) ScaleDeployment(ctx context.Context, namespace, name string, replicas int32) error {
	deployment, err := f.GetDeployment(ctx, namespace, name)
	if err != nil {
		return err
	}
	deployment.Spec.Replicas = &replicas
	_, err = f.clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	return err
}

func (f *FakeK8sClient) UpdatePodResources(ctx context.Context, namespace, name string, resources corev1.ResourceRequirements) error {
	pod, err := f.GetPod(ctx, namespace, name)
	if err != nil {
		return err
	}

	if len(pod.Spec.Containers) > 0 {
		pod.Spec.Containers[0].Resources = resources
		_, err = f.clientset.CoreV1().Pods(namespace).Update(ctx, pod, metav1.UpdateOptions{})
	}

	return err
}

func (f *FakeK8sClient) IsHealthy() bool {
	return true
}

// AdvancedClient methods
func (f *FakeK8sClient) RollbackDeployment(ctx context.Context, namespace, name string) error {
	_, err := f.GetDeployment(ctx, namespace, name)
	return err
}

func (f *FakeK8sClient) ExpandPVC(ctx context.Context, namespace, name, newSize string) error {
	return nil
}

func (f *FakeK8sClient) DrainNode(ctx context.Context, nodeName string) error {
	return nil
}

func (f *FakeK8sClient) QuarantinePod(ctx context.Context, namespace, name string) error {
	_, err := f.GetPod(ctx, namespace, name)
	return err
}

func (f *FakeK8sClient) CollectDiagnostics(ctx context.Context, namespace, resource string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"status": "collected",
		"logs":   []string{"log line 1", "log line 2"},
		"events": []string{"event 1", "event 2"},
	}, nil
}

func (f *FakeK8sClient) AuditLogs(ctx context.Context, namespace, resource, scope string) error {
	return nil
}

func (f *FakeK8sClient) BackupData(ctx context.Context, namespace, resource, backupName string) error {
	return nil
}

// Storage & Persistence actions
func (f *FakeK8sClient) CleanupStorage(ctx context.Context, namespace, podName, path string) error {
	return nil
}

func (f *FakeK8sClient) CompactStorage(ctx context.Context, namespace, resource string) error {
	return nil
}

// Application Lifecycle actions
func (f *FakeK8sClient) CordonNode(ctx context.Context, nodeName string) error {
	return nil
}

func (f *FakeK8sClient) UpdateHPA(ctx context.Context, namespace, name string, minReplicas, maxReplicas int32) error {
	return nil
}

func (f *FakeK8sClient) RestartDaemonSet(ctx context.Context, namespace, name string) error {
	return nil
}

// Security & Compliance actions
func (f *FakeK8sClient) RotateSecrets(ctx context.Context, namespace, secretName string) error {
	return nil
}

// Network & Connectivity actions
func (f *FakeK8sClient) UpdateNetworkPolicy(ctx context.Context, namespace, policyName, actionType string) error {
	return nil
}

func (f *FakeK8sClient) RestartNetwork(ctx context.Context, component string) error {
	return nil
}

func (f *FakeK8sClient) ResetServiceMesh(ctx context.Context, meshType string) error {
	return nil
}

// Database & Stateful actions
func (f *FakeK8sClient) FailoverDatabase(ctx context.Context, namespace, databaseName, replicaName string) error {
	return nil
}

func (f *FakeK8sClient) RepairDatabase(ctx context.Context, namespace, databaseName, repairType string) error {
	return nil
}

func (f *FakeK8sClient) ScaleStatefulSet(ctx context.Context, namespace, name string, replicas int32) error {
	return nil
}

// Monitoring & Observability actions
func (f *FakeK8sClient) EnableDebugMode(ctx context.Context, namespace, resource, logLevel, duration string) error {
	return nil
}

func (f *FakeK8sClient) CreateHeapDump(ctx context.Context, namespace, podName, dumpPath string) error {
	return nil
}

// Resource Management actions
func (f *FakeK8sClient) OptimizeResources(ctx context.Context, namespace, resource, optimizationType string) error {
	return nil
}

func (f *FakeK8sClient) MigrateWorkload(ctx context.Context, namespace, workloadName, targetNode string) error {
	return nil
}

// Helper functions for creating test resources
func createTestDeployment(namespace, name string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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
						"app": name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: "nginx",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("250m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
					},
				},
			},
		},
	}
}

func createTestPod(namespace, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app": "test-app",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "main",
					Image: "nginx",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("250m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
}

var _ = Describe("Executor", func() {
	var (
		logger     *logrus.Logger
		fakeClient *FakeK8sClient
		mockRepo   *SimpleMockRepository
		executor   Executor
		ctx        context.Context
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.FatalLevel)
		mockRepo = &SimpleMockRepository{}
		ctx = context.Background()
	})

	Describe("NewExecutor", func() {
		It("should create a new executor", func() {
			fakeClient = NewFakeK8sClient()
			cfg := config.ActionsConfig{
				DryRun:         false,
				MaxConcurrent:  5,
				CooldownPeriod: 5 * time.Minute,
			}

			executor := NewExecutor(fakeClient, cfg, mockRepo, logger)
			Expect(executor).ToNot(BeNil())
		})
	})

	Describe("IsHealthy", func() {
		It("should return true when healthy", func() {
			fakeClient = NewFakeK8sClient()
			cfg := config.ActionsConfig{
				MaxConcurrent: 1,
			}

			executor := NewExecutor(fakeClient, cfg, mockRepo, logger)
			Expect(executor.IsHealthy()).To(BeTrue())
		})
	})

	Describe("Execute", func() {
		BeforeEach(func() {
			cfg := config.ActionsConfig{
				DryRun:        false,
				MaxConcurrent: 1,
			}
			executor = NewExecutor(fakeClient, cfg, mockRepo, logger)
		})

		Context("scale_deployment action", func() {
			It("should scale deployment successfully", func() {
				deployment := createTestDeployment("test-namespace", "my-app", 3)
				fakeClient = NewFakeK8sClient(deployment)
				executor = NewExecutor(fakeClient, config.ActionsConfig{
					DryRun:        false,
					MaxConcurrent: 1,
				}, mockRepo, logger)

				alert := types.Alert{
					Name:      "HighCPUUsage",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"deployment": "my-app",
					},
				}

				action := &types.ActionRecommendation{
					Action: "scale_deployment",
					Parameters: map[string]interface{}{
						"replicas": 5,
					},
				}

				err := executor.Execute(ctx, action, alert, nil)
				Expect(err).ToNot(HaveOccurred())

				// Verify scaling worked
				updatedDeployment, err := fakeClient.GetDeployment(ctx, "test-namespace", "my-app")
				Expect(err).ToNot(HaveOccurred())
				Expect(*updatedDeployment.Spec.Replicas).To(Equal(int32(5)))
			})
		})

		Context("restart_pod action", func() {
			It("should restart pod successfully", func() {
				pod := createTestPod("test-namespace", "my-app-pod")
				fakeClient = NewFakeK8sClient(pod)
				executor = NewExecutor(fakeClient, config.ActionsConfig{
					DryRun:        false,
					MaxConcurrent: 1,
				}, mockRepo, logger)

				alert := types.Alert{
					Name:      "PodCrashLoop",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"pod": "my-app-pod",
					},
				}

				action := &types.ActionRecommendation{
					Action: "restart_pod",
				}

				err := executor.Execute(ctx, action, alert, nil)
				Expect(err).ToNot(HaveOccurred())

				// Verify pod was deleted
				_, err = fakeClient.GetPod(ctx, "test-namespace", "my-app-pod")
				Expect(err).To(HaveOccurred()) // Pod should be deleted (not found)
			})
		})

		Context("increase_resources action", func() {
			It("should increase resources successfully", func() {
				pod := createTestPod("test-namespace", "my-app-pod")
				fakeClient = NewFakeK8sClient(pod)
				executor = NewExecutor(fakeClient, config.ActionsConfig{
					DryRun:        false,
					MaxConcurrent: 1,
				}, mockRepo, logger)

				alert := types.Alert{
					Name:      "HighMemoryUsage",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"pod": "my-app-pod",
					},
				}

				action := &types.ActionRecommendation{
					Action: "increase_resources",
					Parameters: map[string]interface{}{
						"cpu_limit":      "1000m",
						"memory_limit":   "2Gi",
						"cpu_request":    "500m",
						"memory_request": "1Gi",
					},
				}

				err := executor.Execute(ctx, action, alert, nil)
				Expect(err).ToNot(HaveOccurred())

				// Verify resources were updated
				updatedPod, err := fakeClient.GetPod(ctx, "test-namespace", "my-app-pod")
				Expect(err).ToNot(HaveOccurred())
				// Kubernetes normalizes "1000m" to "1" (1 CPU core)
				Expect(updatedPod.Spec.Containers[0].Resources.Limits.Cpu().String()).To(Equal("1"))
				Expect(updatedPod.Spec.Containers[0].Resources.Limits.Memory().String()).To(Equal("2Gi"))
			})
		})

		Context("notify_only action", func() {
			It("should execute notify_only successfully", func() {
				fakeClient = NewFakeK8sClient()
				executor = NewExecutor(fakeClient, config.ActionsConfig{
					MaxConcurrent: 1,
				}, mockRepo, logger)

				alert := types.Alert{
					Name:      "CriticalAlert",
					Namespace: "production",
				}

				action := &types.ActionRecommendation{
					Action:    "notify_only",
					Reasoning: &types.ReasoningDetails{Summary: "Requires manual intervention"},
					Parameters: map[string]interface{}{
						"message": "Custom notification message",
					},
				}

				err := executor.Execute(ctx, action, alert, nil)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("unknown action", func() {
			It("should return an error for unknown action", func() {
				fakeClient = NewFakeK8sClient()
				executor = NewExecutor(fakeClient, config.ActionsConfig{
					MaxConcurrent: 1,
				}, mockRepo, logger)

				action := &types.ActionRecommendation{
					Action: "unknown_action",
				}

				alert := types.Alert{
					Name:      "TestAlert",
					Namespace: "test",
				}

				err := executor.Execute(ctx, action, alert, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unknown action: unknown_action"))
			})
		})

		Context("dry run mode", func() {
			It("should execute actions without making K8s calls", func() {
				fakeClient = NewFakeK8sClient()
				executor = NewExecutor(fakeClient, config.ActionsConfig{
					DryRun:        true,
					MaxConcurrent: 1,
				}, mockRepo, logger)

				alert := types.Alert{
					Name:      "TestAlert",
					Namespace: "test-namespace",
					Resource:  "my-app-deployment",
				}

				action := &types.ActionRecommendation{
					Action: "scale_deployment",
					Parameters: map[string]interface{}{
						"replicas": 3,
					},
				}

				err := executor.Execute(ctx, action, alert, nil)
				Expect(err).ToNot(HaveOccurred()) // Dry run should always succeed without K8s calls
			})
		})

		Context("parameter handling", func() {
			BeforeEach(func() {
				deployment := createTestDeployment("test-namespace", "my-app", 3)
				fakeClient = NewFakeK8sClient(deployment)
				executor = NewExecutor(fakeClient, config.ActionsConfig{
					DryRun:        false,
					MaxConcurrent: 1,
				}, mockRepo, logger)
			})

			DescribeTable("different parameter types",
				func(params map[string]interface{}, expectErr bool) {
					alert := types.Alert{
						Name:      "HighCPUUsage",
						Namespace: "test-namespace",
						Labels: map[string]string{
							"deployment": "my-app",
						},
					}

					action := &types.ActionRecommendation{
						Action:     "scale_deployment",
						Parameters: params,
					}

					err := executor.Execute(ctx, action, alert, nil)
					if expectErr {
						Expect(err).To(HaveOccurred())
					} else {
						Expect(err).ToNot(HaveOccurred())
					}
				},
				Entry("int replicas", map[string]interface{}{"replicas": 5}, false),
				Entry("float64 replicas", map[string]interface{}{"replicas": 3.0}, false),
				Entry("string replicas", map[string]interface{}{"replicas": "7"}, false),
				Entry("missing replicas", map[string]interface{}{}, true),
			)
		})

		Context("advanced actions", func() {
			Context("rollback_deployment", func() {
				It("should rollback deployment successfully", func() {
					deployment := createTestDeployment("test-namespace", "my-app", 3)
					fakeClient = NewFakeK8sClient(deployment)
					executor = NewExecutor(fakeClient, config.ActionsConfig{
						DryRun:        false,
						MaxConcurrent: 1,
					}, mockRepo, logger)

					alert := types.Alert{
						Name:      "DeploymentFailure",
						Namespace: "test-namespace",
						Labels: map[string]string{
							"deployment": "my-app",
						},
					}

					action := &types.ActionRecommendation{
						Action: "rollback_deployment",
						Parameters: map[string]interface{}{
							"revision": "4",
						},
					}

					err := executor.Execute(ctx, action, alert, nil)
					Expect(err).ToNot(HaveOccurred())

					// Verify deployment still exists (rollback is simulated)
					_, err = fakeClient.GetDeployment(ctx, "test-namespace", "my-app")
					Expect(err).ToNot(HaveOccurred())
				})

				It("should return error for non-existent deployment", func() {
					fakeClient = NewFakeK8sClient() // No deployment created
					executor = NewExecutor(fakeClient, config.ActionsConfig{
						DryRun:        false,
						MaxConcurrent: 1,
					}, mockRepo, logger)

					alert := types.Alert{
						Name:      "DeploymentFailure",
						Namespace: "test-namespace",
						Labels: map[string]string{
							"deployment": "non-existent",
						},
					}

					action := &types.ActionRecommendation{
						Action: "rollback_deployment",
						Parameters: map[string]interface{}{
							"revision": "3",
						},
					}

					err := executor.Execute(ctx, action, alert, nil)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("not found"))
				})
			})

			Context("expand_pvc", func() {
				It("should expand PVC successfully", func() {
					fakeClient = NewFakeK8sClient()
					executor = NewExecutor(fakeClient, config.ActionsConfig{
						DryRun:        false,
						MaxConcurrent: 1,
					}, mockRepo, logger)

					alert := types.Alert{
						Name:      "PVCNearFull",
						Namespace: "test-namespace",
						Resource:  "database-storage",
					}

					action := &types.ActionRecommendation{
						Action: "expand_pvc",
						Parameters: map[string]interface{}{
							"new_size": "20Gi",
						},
					}

					err := executor.Execute(ctx, action, alert, nil)
					Expect(err).ToNot(HaveOccurred())
				})
			})

			Context("drain_node", func() {
				It("should drain node successfully", func() {
					fakeClient = NewFakeK8sClient()
					executor = NewExecutor(fakeClient, config.ActionsConfig{
						DryRun:        false,
						MaxConcurrent: 1,
					}, mockRepo, logger)

					alert := types.Alert{
						Name:     "NodeMaintenanceRequired",
						Resource: "worker-02",
						Labels: map[string]string{
							"node": "worker-02",
						},
					}

					action := &types.ActionRecommendation{
						Action: "drain_node",
						Parameters: map[string]interface{}{
							"force": true,
						},
					}

					err := executor.Execute(ctx, action, alert, nil)
					Expect(err).ToNot(HaveOccurred())
				})
			})

			Context("quarantine_pod", func() {
				It("should quarantine pod successfully", func() {
					pod := createTestPod("test-namespace", "suspicious-pod")
					fakeClient = NewFakeK8sClient(pod)
					executor = NewExecutor(fakeClient, config.ActionsConfig{
						DryRun:        false,
						MaxConcurrent: 1,
					}, mockRepo, logger)

					alert := types.Alert{
						Name:      "SecurityThreatDetected",
						Namespace: "test-namespace",
						Resource:  "suspicious-pod",
					}

					action := &types.ActionRecommendation{
						Action: "quarantine_pod",
						Parameters: map[string]interface{}{
							"reason": "malware_detected",
						},
					}

					err := executor.Execute(ctx, action, alert, nil)
					Expect(err).ToNot(HaveOccurred())

					// Verify pod still exists (quarantine is simulated)
					_, err = fakeClient.GetPod(ctx, "test-namespace", "suspicious-pod")
					Expect(err).ToNot(HaveOccurred())
				})

				It("should return error for non-existent pod", func() {
					fakeClient = NewFakeK8sClient() // No pod created
					executor = NewExecutor(fakeClient, config.ActionsConfig{
						DryRun:        false,
						MaxConcurrent: 1,
					}, mockRepo, logger)

					alert := types.Alert{
						Name:      "SecurityThreatDetected",
						Namespace: "test-namespace",
						Resource:  "non-existent-pod",
					}

					action := &types.ActionRecommendation{
						Action: "quarantine_pod",
						Parameters: map[string]interface{}{
							"reason": "suspicious_activity",
						},
					}

					err := executor.Execute(ctx, action, alert, nil)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("not found"))
				})
			})

			Context("collect_diagnostics", func() {
				It("should collect diagnostics successfully", func() {
					fakeClient = NewFakeK8sClient()
					executor = NewExecutor(fakeClient, config.ActionsConfig{
						DryRun:        false,
						MaxConcurrent: 1,
					}, mockRepo, logger)

					alert := types.Alert{
						Name:      "ComplexServiceFailure",
						Namespace: "test-namespace",
						Resource:  "payment-api-789",
					}

					action := &types.ActionRecommendation{
						Action: "collect_diagnostics",
						Parameters: map[string]interface{}{
							"include_logs":   true,
							"include_events": true,
						},
					}

					err := executor.Execute(ctx, action, alert, nil)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})
	})

	Describe("ActionRegistry Integration", func() {
		It("should have all built-in actions registered", func() {
			fakeClient = NewFakeK8sClient()
			executor := NewExecutor(fakeClient, config.ActionsConfig{
				DryRun:        false,
				MaxConcurrent: 1,
			}, mockRepo, logger)

			registry := executor.GetActionRegistry()
			Expect(registry).ToNot(BeNil())

			expectedActions := []string{
				"scale_deployment",
				"restart_pod",
				"increase_resources",
				"notify_only",
				"rollback_deployment",
				"expand_pvc",
				"drain_node",
				"quarantine_pod",
				"collect_diagnostics",
				"cleanup_storage",
				"backup_data",
				"compact_storage",
				"cordon_node",
				"update_hpa",
				"restart_daemonset",
				"rotate_secrets",
				"audit_logs",
				"update_network_policy",
				"restart_network",
				"reset_service_mesh",
				"failover_database",
				"repair_database",
				"scale_statefulset",
				"enable_debug_mode",
				"create_heap_dump",
				"optimize_resources",
				"migrate_workload",
			}

			registeredActions := registry.GetRegisteredActions()
			Expect(registeredActions).To(HaveLen(len(expectedActions)))

			for _, expectedAction := range expectedActions {
				Expect(registry.IsRegistered(expectedAction)).To(BeTrue(), "Action %s should be registered", expectedAction)
			}
		})

		It("should allow registering custom actions", func() {
			fakeClient = NewFakeK8sClient()
			executor := NewExecutor(fakeClient, config.ActionsConfig{
				DryRun:        false,
				MaxConcurrent: 1,
			}, mockRepo, logger)

			registry := executor.GetActionRegistry()

			// Register a custom action
			customActionExecuted := false
			customHandler := func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
				customActionExecuted = true
				return nil
			}

			err := registry.Register("custom_action", customHandler)
			Expect(err).ToNot(HaveOccurred())

			// Test executing the custom action
			alert := types.Alert{
				Name:      "TestAlert",
				Namespace: "test-namespace",
			}

			action := &types.ActionRecommendation{
				Action: "custom_action",
				Parameters: map[string]interface{}{
					"custom_param": "value",
				},
			}

			err = executor.Execute(ctx, action, alert, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(customActionExecuted).To(BeTrue())
		})
	})
})
