package executor

import (
	"context"
	"testing"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

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

func TestNewExecutor(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	fakeClient := NewFakeK8sClient()
	cfg := config.ActionsConfig{
		DryRun:         false,
		MaxConcurrent:  5,
		CooldownPeriod: 5 * time.Minute,
	}

	executor := NewExecutor(fakeClient, cfg, logger)

	assert.NotNil(t, executor)
	assert.Implements(t, (*Executor)(nil), executor)
}

func TestExecutor_IsHealthy(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	fakeClient := NewFakeK8sClient()
	cfg := config.ActionsConfig{
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	healthy := executor.IsHealthy()

	assert.True(t, healthy)
}

func TestExecutor_Execute_ScaleDeployment(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	// Test successful scaling
	deployment := createTestDeployment("test-namespace", "my-app", 3)
	fakeClient := NewFakeK8sClient(deployment)

	cfg := config.ActionsConfig{
		DryRun:        false,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

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

	err := executor.Execute(ctx, action, alert)
	assert.NoError(t, err)

	// Verify scaling worked
	updatedDeployment, err := fakeClient.GetDeployment(ctx, "test-namespace", "my-app")
	assert.NoError(t, err)
	assert.Equal(t, int32(5), *updatedDeployment.Spec.Replicas)
}

func TestExecutor_Execute_RestartPod(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	pod := createTestPod("test-namespace", "my-app-pod")
	fakeClient := NewFakeK8sClient(pod)

	cfg := config.ActionsConfig{
		DryRun:        false,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

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

	err := executor.Execute(ctx, action, alert)
	assert.NoError(t, err)

	// Verify pod was deleted
	_, err = fakeClient.GetPod(ctx, "test-namespace", "my-app-pod")
	assert.Error(t, err) // Pod should be deleted (not found)
}

func TestExecutor_Execute_IncreaseResources(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	pod := createTestPod("test-namespace", "my-app-pod")
	fakeClient := NewFakeK8sClient(pod)

	cfg := config.ActionsConfig{
		DryRun:        false,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

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

	err := executor.Execute(ctx, action, alert)
	assert.NoError(t, err)

	// Verify resources were updated
	updatedPod, err := fakeClient.GetPod(ctx, "test-namespace", "my-app-pod")
	assert.NoError(t, err)
	// Kubernetes normalizes "1000m" to "1" (1 CPU core)
	assert.Equal(t, "1", updatedPod.Spec.Containers[0].Resources.Limits.Cpu().String())
	assert.Equal(t, "2Gi", updatedPod.Spec.Containers[0].Resources.Limits.Memory().String())
}

func TestExecutor_Execute_NotifyOnly(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	fakeClient := NewFakeK8sClient()
	cfg := config.ActionsConfig{
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

	alert := types.Alert{
		Name:      "CriticalAlert",
		Namespace: "production",
	}

	action := &types.ActionRecommendation{
		Action:    "notify_only",
		Reasoning: "Requires manual intervention",
		Parameters: map[string]interface{}{
			"message": "Custom notification message",
		},
	}

	err := executor.Execute(ctx, action, alert)
	assert.NoError(t, err)
}

func TestExecutor_Execute_UnknownAction(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	fakeClient := NewFakeK8sClient()
	cfg := config.ActionsConfig{
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

	action := &types.ActionRecommendation{
		Action: "unknown_action",
	}

	alert := types.Alert{
		Name:      "TestAlert",
		Namespace: "test",
	}

	err := executor.Execute(ctx, action, alert)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown action: unknown_action")
}

func TestExecutor_Execute_DryRun(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	fakeClient := NewFakeK8sClient()
	cfg := config.ActionsConfig{
		DryRun:        true,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

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

	err := executor.Execute(ctx, action, alert)
	assert.NoError(t, err) // Dry run should always succeed without K8s calls
}

func TestExecutor_Execute_ParameterHandling(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	// Test parameter handling through the Execute method by testing different parameter types
	deployment := createTestDeployment("test-namespace", "my-app", 3)
	fakeClient := NewFakeK8sClient(deployment)

	cfg := config.ActionsConfig{
		DryRun:        false,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

	alert := types.Alert{
		Name:      "HighCPUUsage",
		Namespace: "test-namespace",
		Labels: map[string]string{
			"deployment": "my-app",
		},
	}

	tests := []struct {
		name      string
		params    map[string]interface{}
		expectErr bool
	}{
		{
			name:      "int replicas",
			params:    map[string]interface{}{"replicas": 5},
			expectErr: false,
		},
		{
			name:      "float64 replicas",
			params:    map[string]interface{}{"replicas": 3.0},
			expectErr: false,
		},
		{
			name:      "string replicas",
			params:    map[string]interface{}{"replicas": "7"},
			expectErr: false,
		},
		{
			name:      "missing replicas",
			params:    map[string]interface{}{},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := &types.ActionRecommendation{
				Action:     "scale_deployment",
				Parameters: tt.params,
			}

			err := executor.Execute(ctx, action, alert)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExecutor_Execute_ResourceParameters(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	pod := createTestPod("test-namespace", "my-app-pod")
	fakeClient := NewFakeK8sClient(pod)

	cfg := config.ActionsConfig{
		DryRun:        false,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

	alert := types.Alert{
		Name:      "HighMemoryUsage",
		Namespace: "test-namespace",
		Labels: map[string]string{
			"pod": "my-app-pod",
		},
	}

	// Test with explicit resource parameters
	action := &types.ActionRecommendation{
		Action: "increase_resources",
		Parameters: map[string]interface{}{
			"cpu_limit":      "1000m",
			"memory_limit":   "2Gi",
			"cpu_request":    "500m",
			"memory_request": "1Gi",
		},
	}

	err := executor.Execute(ctx, action, alert)
	assert.NoError(t, err)

	// Verify resources were updated
	updatedPod, err := fakeClient.GetPod(ctx, "test-namespace", "my-app-pod")
	assert.NoError(t, err)
	// Kubernetes normalizes "1000m" to "1" (1 CPU core)
	assert.Equal(t, "1", updatedPod.Spec.Containers[0].Resources.Limits.Cpu().String())
	assert.Equal(t, "2Gi", updatedPod.Spec.Containers[0].Resources.Limits.Memory().String())
}

func TestExecutor_Execute_DeploymentNameResolution(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	tests := []struct {
		name            string
		alert           types.Alert
		expectErr       bool
		setupDeployment bool
	}{
		{
			name: "deployment from labels",
			alert: types.Alert{
				Name:      "HighCPUUsage",
				Namespace: "test-namespace",
				Labels: map[string]string{
					"deployment": "my-deployment",
				},
			},
			expectErr:       false,
			setupDeployment: true,
		},
		{
			name: "deployment from resource",
			alert: types.Alert{
				Name:      "HighCPUUsage",
				Namespace: "test-namespace",
				Resource:  "resource-deployment",
			},
			expectErr:       false,
			setupDeployment: true,
		},
		{
			name: "no deployment name",
			alert: types.Alert{
				Name:      "HighCPUUsage",
				Namespace: "test-namespace",
			},
			expectErr:       true,
			setupDeployment: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fakeClient *FakeK8sClient

			if tt.setupDeployment {
				deploymentName := tt.alert.Labels["deployment"]
				if deploymentName == "" {
					deploymentName = tt.alert.Resource
				}
				deployment := createTestDeployment(tt.alert.Namespace, deploymentName, 3)
				fakeClient = NewFakeK8sClient(deployment)
			} else {
				fakeClient = NewFakeK8sClient()
			}

			cfg := config.ActionsConfig{
				DryRun:        false,
				MaxConcurrent: 1,
			}

			executor := NewExecutor(fakeClient, cfg, logger)
			ctx := context.Background()

			action := &types.ActionRecommendation{
				Action: "scale_deployment",
				Parameters: map[string]interface{}{
					"replicas": 5,
				},
			}

			err := executor.Execute(ctx, action, tt.alert)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
// Tests for Advanced Actions

func TestExecutor_Execute_RollbackDeployment(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	// Test successful rollback
	deployment := createTestDeployment("test-namespace", "my-app", 3)
	fakeClient := NewFakeK8sClient(deployment)
	cfg := config.ActionsConfig{
		DryRun:        false,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

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

	err := executor.Execute(ctx, action, alert)
	assert.NoError(t, err)

	// Verify deployment still exists (rollback is simulated)
	_, err = fakeClient.GetDeployment(ctx, "test-namespace", "my-app")
	assert.NoError(t, err)
}

func TestExecutor_Execute_RollbackDeployment_NonExistentDeployment(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	fakeClient := NewFakeK8sClient() // No deployment created
	cfg := config.ActionsConfig{
		DryRun:        false,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

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

	err := executor.Execute(ctx, action, alert)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestExecutor_Execute_ExpandPVC(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	fakeClient := NewFakeK8sClient()
	cfg := config.ActionsConfig{
		DryRun:        false,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

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

	err := executor.Execute(ctx, action, alert)
	assert.NoError(t, err)
}

func TestExecutor_Execute_ExpandPVC_WithoutParameters(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	fakeClient := NewFakeK8sClient()
	cfg := config.ActionsConfig{
		DryRun:        false,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

	alert := types.Alert{
		Name:      "PVCNearFull",
		Namespace: "test-namespace",
		Resource:  "database-storage",
	}

	action := &types.ActionRecommendation{
		Action:     "expand_pvc",
		Parameters: map[string]interface{}{}, // No parameters
	}

	err := executor.Execute(ctx, action, alert)
	assert.NoError(t, err) // Should still work with default behavior
}

func TestExecutor_Execute_DrainNode(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	fakeClient := NewFakeK8sClient()
	cfg := config.ActionsConfig{
		DryRun:        false,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

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

	err := executor.Execute(ctx, action, alert)
	assert.NoError(t, err)
}

func TestExecutor_Execute_DrainNode_FromResource(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	fakeClient := NewFakeK8sClient()
	cfg := config.ActionsConfig{
		DryRun:        false,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

	alert := types.Alert{
		Name:     "NodeMaintenanceRequired",
		Resource: "worker-03", // Node name from resource field
	}

	action := &types.ActionRecommendation{
		Action:     "drain_node",
		Parameters: map[string]interface{}{},
	}

	err := executor.Execute(ctx, action, alert)
	assert.NoError(t, err)
}

func TestExecutor_Execute_QuarantinePod(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	pod := createTestPod("test-namespace", "suspicious-pod")
	fakeClient := NewFakeK8sClient(pod)
	cfg := config.ActionsConfig{
		DryRun:        false,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

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

	err := executor.Execute(ctx, action, alert)
	assert.NoError(t, err)

	// Verify pod still exists (quarantine is simulated)
	_, err = fakeClient.GetPod(ctx, "test-namespace", "suspicious-pod")
	assert.NoError(t, err)
}

func TestExecutor_Execute_QuarantinePod_NonExistentPod(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	fakeClient := NewFakeK8sClient() // No pod created
	cfg := config.ActionsConfig{
		DryRun:        false,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

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

	err := executor.Execute(ctx, action, alert)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestExecutor_Execute_CollectDiagnostics(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	fakeClient := NewFakeK8sClient()
	cfg := config.ActionsConfig{
		DryRun:        false,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

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

	err := executor.Execute(ctx, action, alert)
	assert.NoError(t, err)
}

func TestExecutor_Execute_CollectDiagnostics_WithoutNamespace(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	fakeClient := NewFakeK8sClient()
	cfg := config.ActionsConfig{
		DryRun:        false,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

	alert := types.Alert{
		Name:     "ClusterWideIssue",
		Resource: "etcd-cluster",
		// No namespace specified
	}

	action := &types.ActionRecommendation{
		Action: "collect_diagnostics",
		Parameters: map[string]interface{}{
			"scope": "cluster",
		},
	}

	err := executor.Execute(ctx, action, alert)
	assert.NoError(t, err)
}

func TestExecutor_Execute_AdvancedActions_DryRun(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	deployment := createTestDeployment("test-namespace", "my-app", 3)
	pod := createTestPod("test-namespace", "my-pod")
	fakeClient := NewFakeK8sClient(deployment, pod)
	
	cfg := config.ActionsConfig{
		DryRun:        true, // Enable dry run
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	ctx := context.Background()

	// Test all advanced actions in dry run mode
	advancedActions := []struct {
		name   string
		action string
		alert  types.Alert
	}{
		{
			name:   "rollback_deployment",
			action: "rollback_deployment",
			alert: types.Alert{
				Name:      "DeploymentFailure",
				Namespace: "test-namespace",
				Labels:    map[string]string{"deployment": "my-app"},
			},
		},
		{
			name:   "expand_pvc",
			action: "expand_pvc",
			alert: types.Alert{
				Name:      "PVCNearFull",
				Namespace: "test-namespace",
				Resource:  "database-storage",
			},
		},
		{
			name:   "drain_node",
			action: "drain_node",
			alert: types.Alert{
				Name:     "NodeMaintenanceRequired",
				Resource: "worker-01",
			},
		},
		{
			name:   "quarantine_pod",
			action: "quarantine_pod",
			alert: types.Alert{
				Name:      "SecurityThreatDetected",
				Namespace: "test-namespace",
				Resource:  "my-pod",
			},
		},
		{
			name:   "collect_diagnostics",
			action: "collect_diagnostics",
			alert: types.Alert{
				Name:      "ComplexServiceFailure",
				Namespace: "test-namespace",
				Resource:  "api-service",
			},
		},
	}

	for _, tt := range advancedActions {
		t.Run(tt.name+"_dry_run", func(t *testing.T) {
			action := &types.ActionRecommendation{
				Action:     tt.action,
				Parameters: map[string]interface{}{},
			}

			err := executor.Execute(ctx, action, tt.alert)
			assert.NoError(t, err) // Dry run should always succeed
		})
	}
}

func TestExecutor_ActionRegistry_Integration(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	fakeClient := NewFakeK8sClient()
	cfg := config.ActionsConfig{
		DryRun:        false,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)

	// Test that the executor implements the interface correctly
	assert.NotNil(t, executor)
	assert.NotNil(t, executor.GetActionRegistry())

	// Test that all built-in actions are registered
	registry := executor.GetActionRegistry()
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
	}

	registeredActions := registry.GetRegisteredActions()
	assert.Len(t, registeredActions, len(expectedActions))

	for _, expectedAction := range expectedActions {
		assert.True(t, registry.IsRegistered(expectedAction), "Action %s should be registered", expectedAction)
	}
}

func TestExecutor_CustomAction_Registration(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	fakeClient := NewFakeK8sClient()
	cfg := config.ActionsConfig{
		DryRun:        false,
		MaxConcurrent: 1,
	}

	executor := NewExecutor(fakeClient, cfg, logger)
	registry := executor.GetActionRegistry()

	// Register a custom action
	customActionExecuted := false
	customHandler := func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
		customActionExecuted = true
		return nil
	}

	err := registry.Register("custom_action", customHandler)
	assert.NoError(t, err)

	// Test executing the custom action
	ctx := context.Background()
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

	err = executor.Execute(ctx, action, alert)
	assert.NoError(t, err)
	assert.True(t, customActionExecuted)
}
