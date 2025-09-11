package testutil

import (
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PlatformTestDataFactory provides standardized test data creation for platform tests
type PlatformTestDataFactory struct{}

// NewPlatformTestDataFactory creates a new test data factory for platform tests
func NewPlatformTestDataFactory() *PlatformTestDataFactory {
	return &PlatformTestDataFactory{}
}

// CreateTestAlert creates a standard test alert
func (f *PlatformTestDataFactory) CreateTestAlert(name, namespace, severity string) types.Alert {
	return types.Alert{
		Name:      name,
		Namespace: namespace,
		Severity:  severity,
		Labels: map[string]string{
			"app":     "test-app",
			"service": "test-service",
		},
		Annotations: map[string]string{
			"description": "Test alert for " + name,
			"summary":     "Test alert summary",
		},
	}
}

// CreateTestActionTrace creates a standard action trace for testing
func (f *PlatformTestDataFactory) CreateTestActionTrace(actionID, actionType, alertName string) *actionhistory.ResourceActionTrace {
	now := time.Now()
	executionStart := now.Add(-20 * time.Minute)
	executionEnd := now.Add(-15 * time.Minute)

	return &actionhistory.ResourceActionTrace{
		ActionID:           actionID,
		ActionType:         actionType,
		AlertName:          alertName,
		ExecutionStartTime: &executionStart,
		ExecutionEndTime:   &executionEnd,
	}
}

// CreateTestPod creates a standard test pod
func (f *PlatformTestDataFactory) CreateTestPod(name, namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":     "test-app",
				"version": "v1.0",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "test-image:latest",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("50m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
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

// CreateTestDeployment creates a standard test deployment
func (f *PlatformTestDataFactory) CreateTestDeployment(name, namespace string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app": "test-app",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-app",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test-app",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "test-container",
							Image: "test-image:latest",
						},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      replicas,
			ReadyReplicas: replicas,
		},
	}
}

// CreateTestService creates a standard test service
func (f *PlatformTestDataFactory) CreateTestService(name, namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "test-app",
			},
			Ports: []corev1.ServicePort{
				{
					Port:     80,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}
}

// CreateTestHPA creates a standard test horizontal pod autoscaler
func (f *PlatformTestDataFactory) CreateTestHPA(name, namespace, targetName string) *autoscalingv1.HorizontalPodAutoscaler {
	minReplicas := int32(2)
	maxReplicas := int32(10)
	targetCPU := int32(80)

	return &autoscalingv1.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: autoscalingv1.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv1.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       targetName,
				APIVersion: "apps/v1",
			},
			MinReplicas:                    &minReplicas,
			MaxReplicas:                    maxReplicas,
			TargetCPUUtilizationPercentage: &targetCPU,
		},
	}
}

// CreateTestNamespace creates a standard test namespace
func (f *PlatformTestDataFactory) CreateTestNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"test": "true",
			},
		},
	}
}

// CreateTestConfigMap creates a standard test config map
func (f *PlatformTestDataFactory) CreateTestConfigMap(name, namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			"config.yaml": "test: configuration",
			"app.config":  "setting=value",
		},
	}
}

// CreateTestSecret creates a standard test secret
func (f *PlatformTestDataFactory) CreateTestSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"username": []byte("testuser"),
			"password": []byte("testpass"),
		},
	}
}

// CreatePrometheusQueryData creates sample Prometheus query response data
func (f *PlatformTestDataFactory) CreatePrometheusQueryData(metricName string, value string) map[string]interface{} {
	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"resultType": "vector",
			"result": []map[string]interface{}{
				{
					"metric": map[string]interface{}{
						"__name__": metricName,
						"job":      "test-job",
						"instance": "test-instance",
					},
					"value": []interface{}{
						time.Now().Unix(),
						value,
					},
				},
			},
		},
	}
}

// CreateMetricsImprovementData creates data showing metrics improvement
func (f *PlatformTestDataFactory) CreateMetricsImprovementData() map[string]string {
	return map[string]string{
		// Memory/CPU metrics: lower values indicate improvement
		"memory_before": "0.8", // High usage before
		"memory_after":  "0.6", // Lower usage after (25% improvement)
		"cpu_before":    "0.9", // High usage before
		"cpu_after":     "0.7", // Lower usage after (22% improvement)

		// Replica metrics: higher values indicate improvement
		"replicas_before": "3", // Lower replicas before
		"replicas_after":  "5", // Higher replicas after (67% improvement)

		// Response time metrics: lower values indicate improvement
		"response_time_before": "500", // High response time before (ms)
		"response_time_after":  "250", // Lower response time after (50% improvement)
	}
}

// CreateAlertManagerAlertData creates sample AlertManager alert data
func (f *PlatformTestDataFactory) CreateAlertManagerAlertData(alertName, status string) map[string]interface{} {
	return map[string]interface{}{
		"status": status,
		"data": []map[string]interface{}{
			{
				"labels": map[string]interface{}{
					"alertname": alertName,
					"namespace": "test-namespace",
					"severity":  "warning",
					"app":       "test-app",
				},
				"annotations": map[string]interface{}{
					"description": "Test alert description",
					"summary":     "Test alert summary",
				},
				"state":    status,
				"activeAt": time.Now().Format(time.RFC3339),
				"value":    "1",
			},
		},
	}
}

// CreateMonitoringEndpoints creates standard monitoring endpoint URLs
func (f *PlatformTestDataFactory) CreateMonitoringEndpoints() map[string]string {
	return map[string]string{
		"prometheus":   "http://prometheus:9090",
		"alertmanager": "http://alertmanager:9093",
		"grafana":      "http://grafana:3000",
	}
}

// CreateExecutorActionParameters creates standard action parameters for executor tests
func (f *PlatformTestDataFactory) CreateExecutorActionParameters(actionType string) map[string]interface{} {
	switch actionType {
	case "scale_deployment":
		return map[string]interface{}{
			"replicas": 3,
			"resource": "deployment/test-app",
		}
	case "restart_pod":
		return map[string]interface{}{
			"resource": "pod/test-pod",
		}
	case "update_hpa":
		return map[string]interface{}{
			"min_replicas": 2,
			"max_replicas": 10,
			"target_cpu":   80,
		}
	default:
		return map[string]interface{}{
			"action": actionType,
		}
	}
}

// CreateTimeRanges creates various time ranges for testing
func (f *PlatformTestDataFactory) CreateTimeRanges() map[string][2]time.Time {
	now := time.Now()
	return map[string][2]time.Time{
		"last_hour":    {now.Add(-time.Hour), now},
		"last_day":     {now.Add(-24 * time.Hour), now},
		"last_week":    {now.Add(-7 * 24 * time.Hour), now},
		"custom_range": {now.Add(-30 * time.Minute), now.Add(-10 * time.Minute)},
	}
}

// Helper function to create int32 pointer
func Int32Ptr(i int32) *int32 {
	return &i
}

// Helper function to create string pointer
func StringPtr(s string) *string {
	return &s
}
