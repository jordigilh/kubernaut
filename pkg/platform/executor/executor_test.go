package executor

import (
	"context"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/jordigilh/kubernaut/test/integration/shared/testenv"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// FakeK8sClient implementation removed - using real fake client from testenv

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
		logger    *logrus.Logger
		k8sClient k8s.Client
		testEnv   *testenv.TestEnvironment
		mockRepo  *SimpleMockRepository
		executor  Executor
		ctx       context.Context
		err       error
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.FatalLevel)
		mockRepo = &SimpleMockRepository{}
		ctx = context.Background()

		// Setup fake K8s environment
		var err error
		testEnv, err = testenv.SetupFakeEnvironment()
		Expect(err).NotTo(HaveOccurred())

		k8sClient = testEnv.CreateK8sClient(logger)
	})

	AfterEach(func() {
		if testEnv != nil {
			err = testEnv.Cleanup()
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Describe("NewExecutor", func() {
		It("should create a new executor", func() {
			cfg := config.ActionsConfig{
				DryRun:         false,
				MaxConcurrent:  5,
				CooldownPeriod: 5 * time.Minute,
			}

			executor, err = NewExecutor(k8sClient, cfg, mockRepo, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(executor).ToNot(BeNil())
		})
	})

	Describe("IsHealthy", func() {
		It("should return true when healthy", func() {
			// Using real K8s client from testEnv
			cfg := config.ActionsConfig{
				MaxConcurrent: 1,
			}

			executor, err = NewExecutor(k8sClient, cfg, mockRepo, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(executor.IsHealthy()).To(BeTrue())
		})
	})

	Describe("Execute", func() {
		BeforeEach(func() {
			cfg := config.ActionsConfig{
				DryRun:        false,
				MaxConcurrent: 1,
			}
			var err error
			executor, err = NewExecutor(k8sClient, cfg, mockRepo, logger)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("scale_deployment action", func() {
			It("should scale deployment successfully", func() {
				deployment := createTestDeployment("test-namespace", "my-app", 3)
				// Create deployment in fake cluster
				_, err = testEnv.Client.AppsV1().Deployments("test-namespace").Create(testEnv.Context, deployment, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				var err error

				executor, err = NewExecutor(k8sClient, config.ActionsConfig{
					DryRun:        false,
					MaxConcurrent: 1,
				}, mockRepo, logger)

				Expect(err).NotTo(HaveOccurred())

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

				err = executor.Execute(ctx, action, alert, nil)
				Expect(err).ToNot(HaveOccurred())

				// Verify scaling worked
				updatedDeployment, err := k8sClient.GetDeployment(ctx, "test-namespace", "my-app")
				Expect(err).ToNot(HaveOccurred())
				Expect(*updatedDeployment.Spec.Replicas).To(Equal(int32(5)))
			})
		})

		Context("restart_pod action", func() {
			It("should restart pod successfully", func() {
				pod := createTestPod("test-namespace", "my-app-pod")
				// Create pod in fake cluster
				_, err = testEnv.Client.CoreV1().Pods("test-namespace").Create(testEnv.Context, pod, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())

				var err error

				executor, err = NewExecutor(k8sClient, config.ActionsConfig{
					DryRun:        false,
					MaxConcurrent: 1,
				}, mockRepo, logger)

				Expect(err).NotTo(HaveOccurred())

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

				err = executor.Execute(ctx, action, alert, nil)
				Expect(err).ToNot(HaveOccurred())

				// Verify pod was deleted
				_, err = k8sClient.GetPod(ctx, "test-namespace", "my-app-pod")
				Expect(err).To(HaveOccurred()) // Pod should be deleted (not found)
			})
		})

		Context("increase_resources action", func() {
			It("should increase resources successfully", func() {
				// Create pod with proper ownership chain (Deployment -> ReplicaSet -> Pod)
				err := testEnv.CreateTestPod("my-app-pod", "test-namespace")
				Expect(err).NotTo(HaveOccurred())

				executor, err = NewExecutor(k8sClient, config.ActionsConfig{
					DryRun:        false,
					MaxConcurrent: 1,
				}, mockRepo, logger)

				Expect(err).NotTo(HaveOccurred())

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

				err = executor.Execute(ctx, action, alert, nil)
				Expect(err).ToNot(HaveOccurred())

				// Verify resources were updated in the deployment template
				// (existing pods won't change, but new pods will use the updated resources)
				updatedDeployment, err := k8sClient.GetDeployment(ctx, "test-namespace", "my-app-pod-deployment")
				Expect(err).ToNot(HaveOccurred())

				// Kubernetes normalizes "1000m" to "1" (1 CPU core)
				Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().String()).To(Equal("1"))
				Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String()).To(Equal("2Gi"))
				Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().String()).To(Equal("500m"))
				Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String()).To(Equal("1Gi"))
			})
		})

		Context("notify_only action", func() {
			It("should execute notify_only successfully", func() {
				// Using real K8s client from testEnv
				var err error

				executor, err = NewExecutor(k8sClient, config.ActionsConfig{
					MaxConcurrent: 1,
				}, mockRepo, logger)

				Expect(err).NotTo(HaveOccurred())

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

				err = executor.Execute(ctx, action, alert, nil)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("unknown action", func() {
			It("should return an error for unknown action", func() {
				// Using real K8s client from testEnv
				var err error

				executor, err = NewExecutor(k8sClient, config.ActionsConfig{
					MaxConcurrent: 1,
				}, mockRepo, logger)

				Expect(err).NotTo(HaveOccurred())

				action := &types.ActionRecommendation{
					Action: "unknown_action",
				}

				alert := types.Alert{
					Name:      "TestAlert",
					Namespace: "test",
				}

				err = executor.Execute(ctx, action, alert, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unknown action: unknown_action"))
			})
		})

		Context("dry run mode", func() {
			It("should execute actions without making K8s calls", func() {
				// Using real K8s client from testEnv
				var err error

				executor, err = NewExecutor(k8sClient, config.ActionsConfig{
					DryRun:        true,
					MaxConcurrent: 1,
				}, mockRepo, logger)

				Expect(err).NotTo(HaveOccurred())

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

				err = executor.Execute(ctx, action, alert, nil)
				Expect(err).ToNot(HaveOccurred()) // Dry run should always succeed without K8s calls
			})
		})

		Context("parameter handling", func() {
			BeforeEach(func() {
				deployment := createTestDeployment("test-namespace", "my-app", 3)
				// Create deployment in fake cluster
				_, err = testEnv.Client.AppsV1().Deployments("test-namespace").Create(testEnv.Context, deployment, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				var err error

				executor, err = NewExecutor(k8sClient, config.ActionsConfig{
					DryRun:        false,
					MaxConcurrent: 1,
				}, mockRepo, logger)

				Expect(err).NotTo(HaveOccurred())
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

					err = executor.Execute(ctx, action, alert, nil)
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
					// Create deployment in fake cluster
					_, err = testEnv.Client.AppsV1().Deployments("test-namespace").Create(testEnv.Context, deployment, metav1.CreateOptions{})
					Expect(err).NotTo(HaveOccurred())
					var err error

					executor, err = NewExecutor(k8sClient, config.ActionsConfig{
						DryRun:        false,
						MaxConcurrent: 1,
					}, mockRepo, logger)

					Expect(err).NotTo(HaveOccurred())

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

					err = executor.Execute(ctx, action, alert, nil)
					Expect(err).ToNot(HaveOccurred())

					// Verify deployment still exists (rollback is simulated)
					_, err = k8sClient.GetDeployment(ctx, "test-namespace", "my-app")
					Expect(err).ToNot(HaveOccurred())
				})

				It("should return error for non-existent deployment", func() {
					// Using real K8s client from testEnv // No deployment created
					var err error

					executor, err = NewExecutor(k8sClient, config.ActionsConfig{
						DryRun:        false,
						MaxConcurrent: 1,
					}, mockRepo, logger)

					Expect(err).NotTo(HaveOccurred())

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

					err = executor.Execute(ctx, action, alert, nil)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("not found"))
				})
			})

			Context("expand_pvc", func() {
				It("should expand PVC successfully", func() {
					// Create a test PVC
					pvc := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "database-storage",
							Namespace: "test-namespace",
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadWriteOnce,
							},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("10Gi"),
								},
							},
						},
					}
					_, err := testEnv.Client.CoreV1().PersistentVolumeClaims("test-namespace").Create(testEnv.Context, pvc, metav1.CreateOptions{})
					Expect(err).NotTo(HaveOccurred())

					// Using real K8s client from testEnv

					executor, err = NewExecutor(k8sClient, config.ActionsConfig{
						DryRun:        false,
						MaxConcurrent: 1,
					}, mockRepo, logger)

					Expect(err).NotTo(HaveOccurred())

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

					err = executor.Execute(ctx, action, alert, nil)
					Expect(err).ToNot(HaveOccurred())
				})
			})

			Context("drain_node", func() {
				It("should drain node successfully", func() {
					// Create a test node
					node := &corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "worker-02",
							Labels: map[string]string{
								"node-role.kubernetes.io/worker": "true",
							},
						},
						Spec: corev1.NodeSpec{},
						Status: corev1.NodeStatus{
							Phase: corev1.NodeRunning,
							Conditions: []corev1.NodeCondition{
								{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								},
							},
						},
					}
					_, err := testEnv.Client.CoreV1().Nodes().Create(testEnv.Context, node, metav1.CreateOptions{})
					Expect(err).NotTo(HaveOccurred())

					// Using real K8s client from testEnv

					executor, err = NewExecutor(k8sClient, config.ActionsConfig{
						DryRun:        false,
						MaxConcurrent: 1,
					}, mockRepo, logger)

					Expect(err).NotTo(HaveOccurred())

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

					err = executor.Execute(ctx, action, alert, nil)
					Expect(err).ToNot(HaveOccurred())
				})
			})

			Context("quarantine_pod", func() {
				It("should quarantine pod successfully", func() {
					pod := createTestPod("test-namespace", "suspicious-pod")
					// Create pod in fake cluster
					_, err = testEnv.Client.CoreV1().Pods("test-namespace").Create(testEnv.Context, pod, metav1.CreateOptions{})
					Expect(err).NotTo(HaveOccurred())
					var err error

					executor, err = NewExecutor(k8sClient, config.ActionsConfig{
						DryRun:        false,
						MaxConcurrent: 1,
					}, mockRepo, logger)

					Expect(err).NotTo(HaveOccurred())

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

					err = executor.Execute(ctx, action, alert, nil)
					Expect(err).ToNot(HaveOccurred())

					// Verify pod still exists (quarantine is simulated)
					_, err = k8sClient.GetPod(ctx, "test-namespace", "suspicious-pod")
					Expect(err).ToNot(HaveOccurred())
				})

				It("should return error for non-existent pod", func() {
					// Using real K8s client from testEnv // No pod created
					var err error

					executor, err = NewExecutor(k8sClient, config.ActionsConfig{
						DryRun:        false,
						MaxConcurrent: 1,
					}, mockRepo, logger)

					Expect(err).NotTo(HaveOccurred())

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

					err = executor.Execute(ctx, action, alert, nil)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("not found"))
				})
			})

			Context("collect_diagnostics", func() {
				It("should collect diagnostics successfully", func() {
					// Using real K8s client from testEnv
					var err error

					executor, err = NewExecutor(k8sClient, config.ActionsConfig{
						DryRun:        false,
						MaxConcurrent: 1,
					}, mockRepo, logger)

					Expect(err).NotTo(HaveOccurred())

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

					err = executor.Execute(ctx, action, alert, nil)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})
	})

	Describe("ActionRegistry Integration", func() {
		It("should have all built-in actions registered", func() {
			// Using real K8s client from testEnv
			executor, err = NewExecutor(k8sClient, config.ActionsConfig{
				DryRun:        false,
				MaxConcurrent: 1,
			}, mockRepo, logger)

			Expect(err).NotTo(HaveOccurred())

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
			// Using real K8s client from testEnv
			executor, err = NewExecutor(k8sClient, config.ActionsConfig{
				DryRun:        false,
				MaxConcurrent: 1,
			}, mockRepo, logger)

			Expect(err).NotTo(HaveOccurred())

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
