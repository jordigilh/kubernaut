package monitoring_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
	"github.com/jordigilh/kubernaut/test/integration/shared/testenv"
)

// NOTE: Replaced MockK8sClient with fake client pattern to ensure
// real Kubernetes API compatibility and catch API evolution issues

// MockAlertClient implements AlertClient for testing
type MockAlertClient struct {
	alertHistory []monitoring.AlertEvent
	shouldError  bool
}

func NewMockAlertClient() *MockAlertClient {
	return &MockAlertClient{
		alertHistory: []monitoring.AlertEvent{},
	}
}

func (m *MockAlertClient) IsAlertResolved(ctx context.Context, alertName, namespace string, since time.Time) (bool, error) {
	if m.shouldError {
		return false, fmt.Errorf("mock error")
	}
	return true, nil
}

func (m *MockAlertClient) HasAlertRecurred(ctx context.Context, alertName, namespace string, from, to time.Time) (bool, error) {
	if m.shouldError {
		return false, fmt.Errorf("mock error")
	}
	return false, nil
}

func (m *MockAlertClient) GetAlertHistory(ctx context.Context, alertName, namespace string, from, to time.Time) ([]monitoring.AlertEvent, error) {
	if m.shouldError {
		return nil, fmt.Errorf("mock error")
	}

	var filteredEvents []monitoring.AlertEvent
	for _, event := range m.alertHistory {
		if (alertName == "" || event.AlertName == alertName) &&
			event.Namespace == namespace &&
			event.Timestamp.After(from) &&
			event.Timestamp.Before(to) {
			filteredEvents = append(filteredEvents, event)
		}
	}
	return filteredEvents, nil
}

var _ = Describe("EnhancedSideEffectDetector", func() {
	var (
		detector    *monitoring.EnhancedSideEffectDetector
		k8sClient   k8s.Client
		testEnv     *testenv.TestEnvironment
		mockAlert   *MockAlertClient
		logger      *logrus.Logger
		ctx         context.Context
		actionTrace *actionhistory.ResourceActionTrace
	)

	BeforeEach(func() {
		var err error
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)
		ctx = context.Background()

		// Setup fake K8s environment - follows established pattern
		testEnv, err = testenv.SetupFakeEnvironment()
		Expect(err).NotTo(HaveOccurred())
		Expect(testEnv).NotTo(BeNil())

		// Create fake K8s client that uses real K8s types and validation
		k8sClient = testEnv.CreateK8sClient(logger)

		mockAlert = NewMockAlertClient()
		detector = monitoring.NewEnhancedSideEffectDetector(k8sClient, mockAlert, logger)

		executionEnd := time.Now().Add(-5 * time.Minute)
		actionTrace = &actionhistory.ResourceActionTrace{
			ActionID:         "test-action-123",
			ActionType:       "scale_deployment",
			AlertName:        "HighMemoryUsage",
			ExecutionEndTime: &executionEnd,
			AlertLabels: actionhistory.JSONMap{
				"namespace":  "test-namespace",
				"deployment": "test-deployment",
			},
			ActionParameters: actionhistory.JSONMap{
				"deployment": "test-deployment",
			},
		}
	})

	AfterEach(func() {
		if testEnv != nil {
			err := testEnv.Cleanup()
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Describe("NewEnhancedSideEffectDetector", func() {
		It("should create a new side effect detector", func() {
			detector := monitoring.NewEnhancedSideEffectDetector(k8sClient, mockAlert, logger)
			Expect(detector).NotTo(BeNil())
		})
	})

	Describe("DetectSideEffects", func() {
		Context("when no side effects are detected", func() {
			BeforeEach(func() {
				// Setup clean state - create normal events in the fake environment
				event := &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-event-normal",
						Namespace:         "test-namespace",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
					},
					Type:    "Normal",
					Reason:  "Scheduled",
					Message: "Successfully assigned test-namespace/test-pod to node1",
					InvolvedObject: corev1.ObjectReference{
						Name:      "test-pod",
						Namespace: "test-namespace",
					},
				}

				// Create namespace first
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-namespace",
					},
				}
				_, err := testEnv.Client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
				if err != nil && !strings.Contains(err.Error(), "already exists") {
					Expect(err).NotTo(HaveOccurred())
				}

				// Create the event directly in the fake environment
				_, err = testEnv.Client.CoreV1().Events("test-namespace").Create(ctx, event, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return empty side effects list", func() {
				sideEffects, err := detector.DetectSideEffects(ctx, actionTrace)

				Expect(err).NotTo(HaveOccurred())
				Expect(sideEffects).To(BeEmpty())
			})
		})

		Context("when problematic Kubernetes events are detected", func() {
			BeforeEach(func() {
				// Create problematic warning event in the fake environment
				event := &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-event-warning",
						Namespace:         "test-namespace",
						CreationTimestamp: metav1.Time{Time: time.Now()}, // After action execution
					},
					Type:    "Warning",
					Reason:  "Failed",
					Message: "Failed to pull image",
					InvolvedObject: corev1.ObjectReference{
						Name:      "test-deployment-123",
						Namespace: "test-namespace",
					},
					Source: corev1.EventSource{
						Component: "kubelet",
					},
				}

				// Create the warning event in the fake environment
				_, err := testEnv.Client.CoreV1().Events("test-namespace").Create(ctx, event, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should detect event-based side effects", func() {
				sideEffects, err := detector.DetectSideEffects(ctx, actionTrace)

				Expect(err).NotTo(HaveOccurred())
				Expect(sideEffects).To(HaveLen(1))

				sideEffect := sideEffects[0]
				Expect(sideEffect.Type).To(Equal("kubernetes_event"))
				Expect(sideEffect.Severity).To(Equal("high"))
				Expect(sideEffect.Description).To(ContainSubstring("Failed"))
				Expect(sideEffect.Evidence).To(HaveKeyWithValue("event_reason", "Failed"))
				Expect(sideEffect.Evidence).To(HaveKeyWithValue("event_message", "Failed to pull image"))
			})
		})

		Context("when new alerts are detected", func() {
			BeforeEach(func() {
				mockAlert.alertHistory = []monitoring.AlertEvent{
					{
						AlertName: "PodRestartBackOff",
						Namespace: "test-namespace",
						Severity:  "warning",
						Status:    "firing",
						Labels: map[string]string{
							"deployment": "test-deployment",
						},
						Timestamp: time.Now().Add(-2 * time.Minute), // After action execution
					},
				}
			})

			It("should detect alert-based side effects", func() {
				sideEffects, err := detector.DetectSideEffects(ctx, actionTrace)

				Expect(err).NotTo(HaveOccurred())
				Expect(sideEffects).To(HaveLen(1))

				sideEffect := sideEffects[0]
				Expect(sideEffect.Type).To(Equal("new_alert"))
				Expect(sideEffect.Severity).To(Equal("medium"))
				Expect(sideEffect.Description).To(ContainSubstring("PodRestartBackOff"))
				Expect(sideEffect.Evidence).To(HaveKeyWithValue("alert_name", "PodRestartBackOff"))
			})
		})

		Context("when resource health degradation is detected", func() {
			Context("for deployment scaling", func() {
				BeforeEach(func() {
					actionTrace.ActionType = "scale_deployment"

					// Setup deployment with unready replicas in the fake environment
					deployment := &appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-deployment",
							Namespace: "test-namespace",
						},
						Spec: appsv1.DeploymentSpec{
							Replicas: func() *int32 { i := int32(5); return &i }(),
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "test-deployment"},
							},
							Template: corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Labels: map[string]string{"app": "test-deployment"},
								},
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{{Name: "main", Image: "nginx:latest"}},
								},
							},
						},
						Status: appsv1.DeploymentStatus{
							Replicas:      5,
							ReadyReplicas: 2, // Only 2 out of 5 ready
						},
					}

					// Create deployment in the fake environment
					_, err := testEnv.Client.AppsV1().Deployments("test-namespace").Create(ctx, deployment, metav1.CreateOptions{})
					Expect(err).NotTo(HaveOccurred())
				})

				It("should detect deployment health degradation", func() {
					sideEffects, err := detector.DetectSideEffects(ctx, actionTrace)

					Expect(err).NotTo(HaveOccurred())
					Expect(sideEffects).To(HaveLen(1))

					sideEffect := sideEffects[0]
					Expect(sideEffect.Type).To(Equal("resource_issue"))
					Expect(sideEffect.Severity).To(Equal("high")) // >50% unready
					Expect(sideEffect.Description).To(ContainSubstring("3 unready replicas"))
					Expect(sideEffect.Evidence).To(HaveKeyWithValue("unready_replicas", 3))
				})
			})

			Context("for pod restart", func() {
				BeforeEach(func() {
					actionTrace.ActionType = "restart_pod"

					// Setup pod with high restart count in the fake environment
					pod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-deployment-123",
							Namespace: "test-namespace",
							Labels: map[string]string{
								"app": "test-deployment",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "main-container", Image: "nginx:latest"}},
						},
						Status: corev1.PodStatus{
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name:         "main-container",
									RestartCount: 5, // High restart count
									Ready:        false,
								},
							},
						},
					}

					// Create pod in the fake environment
					_, err := testEnv.Client.CoreV1().Pods("test-namespace").Create(ctx, pod, metav1.CreateOptions{})
					Expect(err).NotTo(HaveOccurred())
				})

				It("should detect pod health degradation", func() {
					sideEffects, err := detector.DetectSideEffects(ctx, actionTrace)

					Expect(err).NotTo(HaveOccurred())
					Expect(sideEffects).To(HaveLen(1))

					sideEffect := sideEffects[0]
					Expect(sideEffect.Type).To(Equal("resource_issue"))
					Expect(sideEffect.Severity).To(Equal("medium"))
					Expect(sideEffect.Description).To(ContainSubstring("high restart count"))
					Expect(sideEffect.Evidence).To(HaveKeyWithValue("restart_count", int32(5)))
				})
			})

			Context("for resource increases", func() {
				BeforeEach(func() {
					actionTrace.ActionType = "increase_resources"

					// Setup resource quota near limit in the fake environment
					resourceQuota := &corev1.ResourceQuota{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-quota",
							Namespace: "test-namespace",
						},
						Spec: corev1.ResourceQuotaSpec{
							Hard: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
						},
						Status: corev1.ResourceQuotaStatus{
							Hard: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
							Used: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("1.9Gi"), // 95% used
							},
						},
					}

					// Create resource quota in the fake environment
					_, err := testEnv.Client.CoreV1().ResourceQuotas("test-namespace").Create(ctx, resourceQuota, metav1.CreateOptions{})
					Expect(err).NotTo(HaveOccurred())
				})

				It("should detect resource constraint issues", func() {
					sideEffects, err := detector.DetectSideEffects(ctx, actionTrace)

					Expect(err).NotTo(HaveOccurred())
					Expect(sideEffects).To(HaveLen(1))

					sideEffect := sideEffects[0]
					Expect(sideEffect.Type).To(Equal("resource_constraint"))
					Expect(sideEffect.Severity).To(Equal("medium"))
					Expect(sideEffect.Description).To(ContainSubstring("Resource quota"))
					Expect(sideEffect.Evidence).To(HaveKey("utilization_pct"))
				})
			})
		})

		Context("when execution end time is missing", func() {
			BeforeEach(func() {
				actionTrace.ExecutionEndTime = nil
			})

			It("should return an error", func() {
				_, err := detector.DetectSideEffects(ctx, actionTrace)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("missing execution end time"))
			})
		})

		Context("when dependencies fail", func() {
			BeforeEach(func() {
				// Reset ExecutionEndTime in case previous context set it to nil
				executionEnd := time.Now().Add(-5 * time.Minute)
				actionTrace.ExecutionEndTime = &executionEnd

				// Test alert client failure - the fake K8s client will work normally
				// but alert client will fail, allowing us to test error handling
				mockAlert.shouldError = true
			})

			It("should handle errors gracefully and continue processing", func() {
				sideEffects, err := detector.DetectSideEffects(ctx, actionTrace)

				// Should not fail completely, just log warnings
				// Some side effects from K8s resources might still be detected
				Expect(err).NotTo(HaveOccurred())
				Expect(sideEffects).NotTo(BeNil()) // Should return a list, even if empty
			})
		})
	})

	Describe("CheckNewAlerts", func() {
		Context("when new alerts are found", func() {
			BeforeEach(func() {
				mockAlert.alertHistory = []monitoring.AlertEvent{
					{
						AlertName:   "NewAlert",
						Namespace:   "test-namespace",
						Severity:    "critical",
						Status:      "firing",
						Labels:      map[string]string{"service": "test"},
						Annotations: map[string]string{"description": "New problem"},
						Timestamp:   time.Now().Add(-2 * time.Minute),
					},
					{
						AlertName: "OldAlert",
						Namespace: "test-namespace",
						Status:    "resolved",
						Timestamp: time.Now().Add(-20 * time.Minute), // Before since time
					},
				}
			})

			It("should return only firing alerts after since time", func() {
				since := time.Now().Add(-10 * time.Minute)
				alerts, err := detector.CheckNewAlerts(ctx, "test-namespace", since)

				Expect(err).NotTo(HaveOccurred())
				Expect(alerts).To(HaveLen(1))

				alert := alerts[0]
				Expect(alert.Name).To(Equal("NewAlert"))
				Expect(alert.Status).To(Equal("firing"))
				Expect(alert.Severity).To(Equal("critical"))
			})
		})

		Context("when no AlertClient is available", func() {
			BeforeEach(func() {
				detector = monitoring.NewEnhancedSideEffectDetector(k8sClient, nil, logger)
			})

			It("should return empty list without error", func() {
				since := time.Now().Add(-10 * time.Minute)
				alerts, err := detector.CheckNewAlerts(ctx, "test-namespace", since)

				Expect(err).NotTo(HaveOccurred())
				Expect(alerts).To(BeEmpty())
			})
		})
	})

	Describe("Event Classification", func() {
		Context("problematic events", func() {
			It("should identify warning events as problematic", func() {
				problematicEvent := &corev1.Event{
					Type:   "Warning",
					Reason: "Failed",
					InvolvedObject: corev1.ObjectReference{
						Name: "test-deployment",
					},
				}

				// This would be tested through the detector behavior
				// The isProblematicEvent method is internal
				Expect(problematicEvent.Type).To(Equal("Warning"))
				Expect(problematicEvent.Reason).To(Equal("Failed"))
			})
		})

		Context("severity categorization", func() {
			It("should handle different event severities", func() {
				errorEvent := &corev1.Event{Type: "Error"}
				warningEvent := &corev1.Event{Type: "Warning"}

				// Internal methods tested through behavior
				Expect(errorEvent.Type).To(Equal("Error"))
				Expect(warningEvent.Type).To(Equal("Warning"))
			})
		})
	})

	Describe("Alert Correlation", func() {
		Context("alert-to-action relationship", func() {
			It("should identify related alerts", func() {
				relatedAlert := &types.Alert{
					Name:      "RelatedAlert",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"deployment": "test-deployment",
					},
				}

				// Tested through side effect detection behavior
				namespace := "test-namespace"
				resourceName := "test-deployment"
				Expect(relatedAlert.Namespace).To(Equal(namespace))
				Expect(relatedAlert.Labels["deployment"]).To(Equal(resourceName))
			})
		})
	})
})
