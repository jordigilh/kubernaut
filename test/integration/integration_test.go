//go:build integration
// +build integration

package integration

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/executor"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/slm"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/jordigilh/prometheus-alerts-slm/test/integration/shared"
	"github.com/jordigilh/prometheus-alerts-slm/test/integration/shared/testenv"
)

var _ = Describe("SLM Integration", func() {
	It("should successfully analyze alerts with SLM", func() {
		// Skip if Ollama is not available
		if os.Getenv("SKIP_INTEGRATION") != "" {
			Skip("Integration tests skipped")
		}

		// Create logger
		logger := logrus.New()
		logger.SetLevel(logrus.DebugLevel)

		// Create SLM configuration
		cfg := config.SLMConfig{
			Provider:    "localai",
			Endpoint:    "http://localhost:11434",
			Model:       "granite3.1-dense:8b",
			Temperature: 0.3,
			MaxTokens:   500,
			Timeout:     30 * time.Second,
			RetryCount:  2,
		}

		// Create SLM client
		client, err := slm.NewClient(cfg, logger)
		Expect(err).ToNot(HaveOccurred())

		// Test health check
		if !client.IsHealthy() {
			Skip("Ollama is not healthy, skipping integration test")
		}

		// Create test alert
		testAlert := types.Alert{
			Name:        "HighMemoryUsage",
			Status:      "firing",
			Severity:    "warning",
			Description: "Pod is using 95% of memory limit",
			Namespace:   "production",
			Resource:    "web-app-pod-123",
			Labels: map[string]string{
				"alertname":  "HighMemoryUsage",
				"severity":   "warning",
				"namespace":  "production",
				"pod":        "web-app-pod-123",
				"deployment": "web-app",
			},
			Annotations: map[string]string{
				"description": "Pod is using 95% of memory limit",
				"summary":     "High memory usage detected",
			},
			StartsAt: time.Now().Add(-5 * time.Minute),
		}

		// Test alert analysis
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		recommendation, err := client.AnalyzeAlert(ctx, testAlert)
		Expect(err).ToNot(HaveOccurred())

		// Validate recommendation
		Expect(recommendation.Action).ToNot(BeEmpty())

		validActions := []string{"scale_deployment", "restart_pod", "increase_resources", "notify_only"}
		Expect(recommendation.Action).To(BeElementOf(validActions))

		Expect(recommendation.Confidence).To(BeNumerically(">=", 0.0))
		Expect(recommendation.Confidence).To(BeNumerically("<=", 1.0))

		GinkgoWriter.Printf("Integration test successful - Action: %s, Confidence: %.2f\n",
			recommendation.Action, recommendation.Confidence)
	})
})

var _ = Describe("Kubernetes Integration", func() {
	var (
		testEnv *shared.TestEnvironment
		logger  *logrus.Logger
	)

	BeforeEach(func() {
		// Skip if K8s integration is disabled
		if os.Getenv("SKIP_K8S_INTEGRATION") != "" {
			Skip("Kubernetes integration tests skipped")
		}

		// Create logger
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		// Setup fake Kubernetes environment
		var err error
		testEnv, err = testenv.SetupFakeEnvironment()
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if testEnv != nil {
			err := testEnv.Cleanup()
			Expect(err).ToNot(HaveOccurred())
		}
	})

	It("should create and manage Kubernetes resources", func() {
		// Create a test namespace
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-namespace",
			},
		}

		createdNS, err := testEnv.Client.CoreV1().Namespaces().Create(
			testEnv.Context, namespace, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(createdNS.Name).To(Equal("test-namespace"))

		// Create a test deployment
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deployment",
				Namespace: "test-namespace",
				Labels:    map[string]string{"app": "test"},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(3),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "test"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "test-container",
								Image: "nginx:latest",
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
							},
						},
					},
				},
			},
		}

		createdDep, err := testEnv.Client.AppsV1().Deployments("test-namespace").Create(
			testEnv.Context, deployment, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(createdDep.Name).To(Equal("test-deployment"))
		Expect(*createdDep.Spec.Replicas).To(Equal(int32(3)))

		// Verify the deployment can be retrieved
		retrievedDep, err := testEnv.Client.AppsV1().Deployments("test-namespace").Get(
			testEnv.Context, "test-deployment", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(retrievedDep.Name).To(Equal("test-deployment"))

		GinkgoWriter.Printf("Successfully created and verified deployment: %s\n", retrievedDep.Name)
	})

	It("should test K8s client interface", func() {
		// Create k8s client using testenv
		k8sClient := testEnv.CreateK8sClient(logger)
		Expect(k8sClient).ToNot(BeNil())

		// Create a test namespace directly using the client
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "client-test-ns",
			},
		}

		_, err := testEnv.Client.CoreV1().Namespaces().Create(
			testEnv.Context, namespace, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// Verify namespace was created
		ns, err := testEnv.Client.CoreV1().Namespaces().Get(
			testEnv.Context, "client-test-ns", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(ns.Name).To(Equal("client-test-ns"))

		GinkgoWriter.Printf("Successfully tested k8s client interface with namespace: %s\n", ns.Name)
	})
})

// Helper function for int32 pointer
func int32Ptr(i int32) *int32 {
	return &i
}

var _ = Describe("End-to-End Flow", func() {
	var (
		testEnv   *testenv.TestEnvironment
		logger    *logrus.Logger
		slmClient slm.Client
		exec      executor.Executor
	)

	BeforeEach(func() {
		// Skip if components are not available
		if os.Getenv("SKIP_E2E") != "" {
			Skip("End-to-end tests skipped")
		}

		// Create logger
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		// Setup fake Kubernetes environment
		var err error
		testEnv, err = testenv.SetupFakeEnvironment()
		Expect(err).ToNot(HaveOccurred())

		// Setup SLM client (only if not skipping integration)
		if os.Getenv("SKIP_INTEGRATION") == "" {
			cfg := config.SLMConfig{
				Provider:    "localai",
				Endpoint:    "http://localhost:11434",
				Model:       "granite3.1-dense:8b",
				Temperature: 0.3,
				MaxTokens:   500,
				Timeout:     30 * time.Second,
				RetryCount:  1,
			}

			slmClient, err = slm.NewClient(cfg, logger)
			if err != nil {
				Skip("Failed to create SLM client, skipping E2E test")
			}

			if !slmClient.IsHealthy() {
				Skip("SLM is not healthy, skipping E2E test")
			}
		}

		// Create executor with K8s client
		k8sClient := testEnv.CreateK8sClient(logger)
		exec = executor.NewExecutor(k8sClient, config.ActionsConfig{
			DryRun:         false,
			MaxConcurrent:  5,
			CooldownPeriod: 30 * time.Second,
		}, logger)
	})

	AfterEach(func() {
		if testEnv != nil {
			err := testEnv.Cleanup()
			Expect(err).ToNot(HaveOccurred())
		}
	})

	It("should process complete alert and action flow", func() {
		// 1. Create a test deployment in K8s
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-app",
				Namespace: "default",
				Labels:    map[string]string{"app": "web-app"},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(2),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "web-app"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "web-app"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "web-container",
								Image: "nginx:latest",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("128Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("200m"),
										corev1.ResourceMemory: resource.MustParse("256Mi"),
									},
								},
							},
						},
					},
				},
			},
		}

		_, err := testEnv.Client.AppsV1().Deployments("default").Create(
			testEnv.Context, deployment, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// 2. Create test alert for high memory usage
		testAlert := types.Alert{
			Name:        "HighMemoryUsage",
			Status:      "firing",
			Severity:    "warning",
			Description: "Pod is using 90% of memory limit",
			Namespace:   "default",
			Resource:    "web-app",
			Labels: map[string]string{
				"alertname":  "HighMemoryUsage",
				"severity":   "warning",
				"namespace":  "default",
				"deployment": "web-app",
			},
			Annotations: map[string]string{
				"description": "Pod is using 90% of memory limit",
				"summary":     "High memory usage detected",
			},
			StartsAt: time.Now().Add(-5 * time.Minute),
		}

		// 3. Analyze alert with SLM (if available)
		var recommendation *types.ActionRecommendation
		if slmClient != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			recommendation, err = slmClient.AnalyzeAlert(ctx, testAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(recommendation).ToNot(BeNil())
			Expect(recommendation.Action).ToNot(BeEmpty())

			GinkgoWriter.Printf("SLM recommended action: %s (confidence: %.2f)\n",
				recommendation.Action, recommendation.Confidence)
		} else {
			// Create a mock recommendation for testing executor
			recommendation = &types.ActionRecommendation{
				Action:     "scale_deployment",
				Confidence: 0.8,
				Reasoning: &types.ReasoningDetails{
					Summary: "Scale deployment due to high memory usage",
				},
				Parameters: map[string]interface{}{
					"replicas": 4,
				},
			}
		}

		// 4. Execute action via executor
		actionCtx, actionCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer actionCancel()

		err = exec.Execute(actionCtx, recommendation, testAlert)
		Expect(err).ToNot(HaveOccurred())

		// 5. Verify the action was executed in K8s
		if recommendation.Action == "scale_deployment" {
			// Verify deployment was scaled
			updatedDep, err := testEnv.Client.AppsV1().Deployments("default").Get(
				testEnv.Context, "web-app", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())

			expectedReplicas := int32(4)
			if replicas, ok := recommendation.Parameters["replicas"].(int); ok {
				expectedReplicas = int32(replicas)
			}
			Expect(*updatedDep.Spec.Replicas).To(Equal(expectedReplicas))

			GinkgoWriter.Printf("Successfully scaled deployment to %d replicas\n", *updatedDep.Spec.Replicas)
		}

		GinkgoWriter.Printf("End-to-end test completed successfully: %s\n", recommendation.Action)
	})
})
