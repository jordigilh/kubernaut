//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/executor"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/slm"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	testNamespace  = "e2e-test"
	testDeployment = "test-app-e2e"
	kindContext    = "kind-prometheus-alerts-slm-test"
)

func TestKindClusterOperations(t *testing.T) {
	if os.Getenv("SKIP_KIND_E2E") != "" {
		t.Skip("KinD e2e tests skipped")
	}

	// Check if we're running in KinD context
	if !isKindClusterAvailable(t) {
		t.Skip("KinD cluster not available, run: ./scripts/setup-kind-cluster.sh")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create MCP client for KinD cluster
	mcpConfig := config.OpenShiftConfig{
		Context:   kindContext,
		Namespace: testNamespace,
	}

	mcpClient, err := mcp.NewClient(mcpConfig, logger)
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}

	// Test MCP client health
	if !mcpClient.IsHealthy() {
		t.Fatalf("MCP client is not healthy")
	}

	t.Run("TestDeploymentOperations", func(t *testing.T) {
		testDeploymentOperations(t, mcpClient)
	})

	t.Run("TestPodOperations", func(t *testing.T) {
		testPodOperations(t, mcpClient)
	})

	t.Run("TestExecutorIntegration", func(t *testing.T) {
		testExecutorIntegration(t, logger)
	})
}

func testDeploymentOperations(t *testing.T, mcpClient mcp.Client) {
	ctx := context.Background()

	// Create test deployment
	deployment := createTestDeployment()
	
	// Get kubernetes client to create deployment
	kubeClient := getKubernetesClient(t)
	
	// Create deployment
	_, err := kubeClient.AppsV1().Deployments(testNamespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create test deployment: %v", err)
	}
	
	// Cleanup
	defer func() {
		kubeClient.AppsV1().Deployments(testNamespace).Delete(ctx, testDeployment, metav1.DeleteOptions{})
	}()

	// Wait for deployment to be ready
	waitForDeployment(t, kubeClient, testNamespace, testDeployment)

	// Test getting deployment
	dep, err := mcpClient.GetDeployment(ctx, testNamespace, testDeployment)
	if err != nil {
		t.Fatalf("Failed to get deployment: %v", err)
	}

	if dep.Name != testDeployment {
		t.Errorf("Expected deployment name %s, got %s", testDeployment, dep.Name)
	}

	// Test scaling deployment
	originalReplicas := *dep.Spec.Replicas
	newReplicas := originalReplicas + 1

	err = mcpClient.ScaleDeployment(ctx, testNamespace, testDeployment, newReplicas)
	if err != nil {
		t.Fatalf("Failed to scale deployment: %v", err)
	}

	// Verify scaling
	time.Sleep(2 * time.Second)
	scaledDep, err := mcpClient.GetDeployment(ctx, testNamespace, testDeployment)
	if err != nil {
		t.Fatalf("Failed to get scaled deployment: %v", err)
	}

	if *scaledDep.Spec.Replicas != newReplicas {
		t.Errorf("Expected %d replicas, got %d", newReplicas, *scaledDep.Spec.Replicas)
	}

	t.Logf("Successfully scaled deployment from %d to %d replicas", originalReplicas, newReplicas)
}

func testPodOperations(t *testing.T, mcpClient mcp.Client) {
	ctx := context.Background()

	// List pods in test namespace
	pods, err := mcpClient.ListPodsWithLabel(ctx, testNamespace, "app=test-app-e2e")
	if err != nil {
		t.Fatalf("Failed to list pods: %v", err)
	}

	if len(pods.Items) == 0 {
		t.Skip("No pods found for testing, skipping pod operations")
	}

	podName := pods.Items[0].Name

	// Test getting pod
	pod, err := mcpClient.GetPod(ctx, testNamespace, podName)
	if err != nil {
		t.Fatalf("Failed to get pod: %v", err)
	}

	if pod.Name != podName {
		t.Errorf("Expected pod name %s, got %s", podName, pod.Name)
	}

	t.Logf("Successfully retrieved pod: %s", podName)

	// Note: We don't test pod deletion in e2e as it would disrupt the deployment
	// In real scenarios, pod deletion would be tested with dedicated test pods
}

func testExecutorIntegration(t *testing.T, logger *logrus.Logger) {
	// Test executor with KinD cluster
	executorConfig := config.ActionsConfig{
		DryRun:          true, // Always use dry-run in tests
		MaxConcurrent:   1,
		CooldownPeriod:  1 * time.Minute,
	}

	mcpConfig := config.OpenShiftConfig{
		Context:   kindContext,
		Namespace: testNamespace,
	}

	// Create MCP client
	mcpClient, err := mcp.NewClient(mcpConfig, logger)
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}

	// Create executor
	exec := executor.NewExecutor(mcpClient, executorConfig, logger)

	// Create test action recommendation
	actionRecommendation := &types.ActionRecommendation{
		Action: "scale_deployment",
		Parameters: map[string]interface{}{
			"replicas": 3,
		},
		Confidence: 0.95,
		Reasoning:  "E2E test scaling action",
	}

	// Create test alert
	testAlert := types.Alert{
		Name:        "TestDeploymentScale",
		Namespace:   testNamespace,
		Resource:    testDeployment,
		Severity:    "warning",
		Description: "Test alert for scaling",
		Labels: map[string]string{
			"deployment": testDeployment,
		},
	}

	ctx := context.Background()
	err = exec.Execute(ctx, actionRecommendation, testAlert)
	if err != nil {
		t.Fatalf("Failed to execute scale action: %v", err)
	}

	t.Log("Successfully executed scale action in dry-run mode")
}

func isKindClusterAvailable(t *testing.T) bool {
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		return false
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	return err == nil
}

func getKubernetesClient(t *testing.T) kubernetes.Interface {
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		t.Fatalf("Failed to get kubernetes config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatalf("Failed to create kubernetes client: %v", err)
	}

	return clientset
}

func createTestDeployment() *appsv1.Deployment {
	replicas := int32(2)
	
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testDeployment,
			Namespace: testNamespace,
			Labels: map[string]string{
				"app": "test-app-e2e",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-app-e2e",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test-app-e2e",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:1.21",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 80,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("64Mi"),
									corev1.ResourceCPU:    resource.MustParse("50m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
									corev1.ResourceCPU:    resource.MustParse("100m"),
								},
							},
						},
					},
				},
			},
		},
	}
}

func waitForDeployment(t *testing.T, client kubernetes.Interface, namespace, name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for deployment %s to be ready", name)
		case <-time.After(5 * time.Second):
			dep, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				continue
			}
			
			if dep.Status.ReadyReplicas == *dep.Spec.Replicas {
				t.Logf("Deployment %s is ready with %d replicas", name, dep.Status.ReadyReplicas)
				return
			}
			
			t.Logf("Waiting for deployment %s: %d/%d replicas ready", 
				name, dep.Status.ReadyReplicas, *dep.Spec.Replicas)
		}
	}
}