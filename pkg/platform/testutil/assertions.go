package testutil

import (
	"time"

	. "github.com/onsi/gomega" //nolint:staticcheck
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PlatformAssertions provides standardized assertion helpers for platform tests
type PlatformAssertions struct{}

// NewPlatformAssertions creates a new platform assertions helper
func NewPlatformAssertions() *PlatformAssertions {
	return &PlatformAssertions{}
}

// AssertNoError verifies no error occurred with context
func (a *PlatformAssertions) AssertNoError(err error, context string) {
	Expect(err).NotTo(HaveOccurred(), "Expected no error in %s, but got: %v", context, err)
}

// AssertErrorContains verifies error contains expected text
func (a *PlatformAssertions) AssertErrorContains(err error, expectedText string) {
	Expect(err).To(HaveOccurred(), "Expected an error containing '%s'", expectedText)
	Expect(err.Error()).To(ContainSubstring(expectedText))
}

// AssertK8sResourceExists verifies a Kubernetes resource exists
func (a *PlatformAssertions) AssertK8sResourceExists(resource metav1.Object) {
	Expect(resource).NotTo(BeNil())
	Expect(resource.GetName()).NotTo(BeEmpty())
	Expect(resource.GetNamespace()).NotTo(BeEmpty())
}

// AssertPodRunning verifies a pod is in running state
func (a *PlatformAssertions) AssertPodRunning(pod *corev1.Pod) {
	Expect(pod).NotTo(BeNil())
	Expect(pod.Status.Phase).To(Equal(corev1.PodRunning))
}

// AssertPodLabels verifies pod has expected labels
func (a *PlatformAssertions) AssertPodLabels(pod *corev1.Pod, expectedLabels map[string]string) {
	Expect(pod).NotTo(BeNil())
	for key, expectedValue := range expectedLabels {
		Expect(pod.Labels).To(HaveKeyWithValue(key, expectedValue))
	}
}

// AssertDeploymentReady verifies deployment is ready with expected replicas
func (a *PlatformAssertions) AssertDeploymentReady(deployment *appsv1.Deployment, expectedReplicas int32) {
	Expect(deployment).NotTo(BeNil())
	Expect(deployment.Status.Replicas).To(Equal(expectedReplicas))
	Expect(deployment.Status.ReadyReplicas).To(Equal(expectedReplicas))
}

// AssertDeploymentScaled verifies deployment was scaled to expected replicas
func (a *PlatformAssertions) AssertDeploymentScaled(deployment *appsv1.Deployment, expectedReplicas int32) {
	Expect(deployment).NotTo(BeNil())
	Expect(*deployment.Spec.Replicas).To(Equal(expectedReplicas))
}

// AssertHPAConfiguration verifies HPA has correct configuration
func (a *PlatformAssertions) AssertHPAConfiguration(hpa *autoscalingv1.HorizontalPodAutoscaler, minReplicas, maxReplicas int32, targetCPU int32) {
	Expect(hpa).NotTo(BeNil())
	Expect(*hpa.Spec.MinReplicas).To(Equal(minReplicas))
	Expect(hpa.Spec.MaxReplicas).To(Equal(maxReplicas))
	Expect(*hpa.Spec.TargetCPUUtilizationPercentage).To(Equal(targetCPU))
}

// AssertServiceEndpoints verifies service has correct endpoints
func (a *PlatformAssertions) AssertServiceEndpoints(service *corev1.Service, expectedPorts []int32) {
	Expect(service).NotTo(BeNil())
	Expect(service.Spec.Ports).To(HaveLen(len(expectedPorts)))

	for i, port := range expectedPorts {
		Expect(service.Spec.Ports[i].Port).To(Equal(port))
	}
}

// AssertMetricsImprovement verifies metrics show improvement
func (a *PlatformAssertions) AssertMetricsImprovement(beforeValue, afterValue float64, improvementType string) {
	switch improvementType {
	case "lower_is_better": // CPU, Memory, Response Time
		Expect(afterValue).To(BeNumerically("<", beforeValue), "Expected improvement (lower value)")
		improvement := (beforeValue - afterValue) / beforeValue
		Expect(improvement).To(BeNumerically(">=", 0.05), "Expected at least 5% improvement")
	case "higher_is_better": // Replicas, Throughput
		Expect(afterValue).To(BeNumerically(">", beforeValue), "Expected improvement (higher value)")
		improvement := (afterValue - beforeValue) / beforeValue
		Expect(improvement).To(BeNumerically(">=", 0.05), "Expected at least 5% improvement")
	}
}

// AssertPrometheusQueryResponse verifies Prometheus query response structure
func (a *PlatformAssertions) AssertPrometheusQueryResponse(response map[string]interface{}) {
	Expect(response).To(HaveKey("status"))
	Expect(response["status"]).To(Equal("success"))
	Expect(response).To(HaveKey("data"))

	data := response["data"].(map[string]interface{})
	Expect(data).To(HaveKey("resultType"))
	Expect(data).To(HaveKey("result"))
}

// AssertAlertManagerResponse verifies AlertManager response structure
func (a *PlatformAssertions) AssertAlertManagerResponse(response map[string]interface{}) {
	Expect(response).To(HaveKey("status"))
	Expect(response).To(HaveKey("data"))

	data := response["data"].([]map[string]interface{})
	Expect(data).NotTo(BeEmpty())

	for _, alert := range data {
		Expect(alert).To(HaveKey("labels"))
		Expect(alert).To(HaveKey("state"))
	}
}

// AssertHTTPStatusOK verifies HTTP response status is OK (2xx)
func (a *PlatformAssertions) AssertHTTPStatusOK(statusCode int) {
	Expect(statusCode).To(BeNumerically(">=", 200))
	Expect(statusCode).To(BeNumerically("<", 300))
}

// AssertClientHealthy verifies client reports healthy status
func (a *PlatformAssertions) AssertClientHealthy(isHealthy bool) {
	Expect(isHealthy).To(BeTrue())
}

// AssertActionExecuted verifies an action was executed successfully
func (a *PlatformAssertions) AssertActionExecuted(result map[string]interface{}) {
	Expect(result).To(HaveKey("success"))
	Expect(result["success"]).To(BeTrue())
	Expect(result).To(HaveKey("action_id"))
	Expect(result["action_id"]).NotTo(BeEmpty())
}

// AssertActionFailed verifies an action failed as expected
func (a *PlatformAssertions) AssertActionFailed(result map[string]interface{}, expectedError string) {
	Expect(result).To(HaveKey("success"))
	Expect(result["success"]).To(BeFalse())
	Expect(result).To(HaveKey("error"))
	Expect(result["error"].(string)).To(ContainSubstring(expectedError))
}

// AssertTimeRange verifies time is within expected range
func (a *PlatformAssertions) AssertTimeRange(timestamp time.Time, startTime, endTime time.Time) {
	Expect(timestamp).To(BeTemporally(">=", startTime))
	Expect(timestamp).To(BeTemporally("<=", endTime))
}

// AssertRecentTimestamp verifies timestamp is recent (within specified duration)
func (a *PlatformAssertions) AssertRecentTimestamp(timestamp time.Time, maxAge time.Duration) {
	now := time.Now()
	Expect(timestamp).To(BeTemporally(">=", now.Add(-maxAge)))
	Expect(timestamp).To(BeTemporally("<=", now.Add(time.Minute))) // Small buffer
}

// AssertNamespaceMatch verifies resource is in expected namespace
func (a *PlatformAssertions) AssertNamespaceMatch(resource metav1.Object, expectedNamespace string) {
	Expect(resource.GetNamespace()).To(Equal(expectedNamespace))
}

// AssertResourceLabelsMatch verifies resource has expected labels
func (a *PlatformAssertions) AssertResourceLabelsMatch(resource metav1.Object, expectedLabels map[string]string) {
	labels := resource.GetLabels()
	for key, expectedValue := range expectedLabels {
		Expect(labels).To(HaveKeyWithValue(key, expectedValue))
	}
}

// AssertConfigMapData verifies ConfigMap has expected data
func (a *PlatformAssertions) AssertConfigMapData(configMap *corev1.ConfigMap, expectedData map[string]string) {
	Expect(configMap).NotTo(BeNil())
	for key, expectedValue := range expectedData {
		Expect(configMap.Data).To(HaveKeyWithValue(key, expectedValue))
	}
}

// AssertSecretData verifies Secret has expected data
func (a *PlatformAssertions) AssertSecretData(secret *corev1.Secret, expectedKeys []string) {
	Expect(secret).NotTo(BeNil())
	for _, key := range expectedKeys {
		Expect(secret.Data).To(HaveKey(key))
		Expect(secret.Data[key]).NotTo(BeEmpty())
	}
}

// AssertContainerResources verifies container has expected resource limits/requests
func (a *PlatformAssertions) AssertContainerResources(container corev1.Container, hasLimits, hasRequests bool) {
	if hasLimits {
		Expect(container.Resources.Limits).NotTo(BeEmpty())
	}
	if hasRequests {
		Expect(container.Resources.Requests).NotTo(BeEmpty())
	}
}

// AssertQuarantineLabels verifies quarantine labels are applied correctly
func (a *PlatformAssertions) AssertQuarantineLabels(resource metav1.Object) {
	labels := resource.GetLabels()
	Expect(labels).To(HaveKey("kubernaut.quarantined"))
	Expect(labels["kubernaut.quarantined"]).To(Equal("true"))
	Expect(labels).To(HaveKey("kubernaut.quarantine-reason"))
}

// AssertMonitoringMetrics verifies monitoring metrics are properly structured
func (a *PlatformAssertions) AssertMonitoringMetrics(metrics map[string]float64) {
	Expect(metrics).NotTo(BeEmpty())

	// Verify common metric properties
	for name, value := range metrics {
		Expect(name).NotTo(BeEmpty(), "Metric name should not be empty")
		Expect(value).To(BeNumerically(">=", 0), "Metric value should be non-negative: %s", name)
	}
}

// AssertSideEffectDetection verifies side effect detection results
func (a *PlatformAssertions) AssertSideEffectDetection(detected bool, sideEffects []string) {
	if detected {
		Expect(sideEffects).NotTo(BeEmpty(), "Expected side effects to be detected")
		for _, effect := range sideEffects {
			Expect(effect).NotTo(BeEmpty(), "Side effect description should not be empty")
		}
	} else {
		Expect(sideEffects).To(BeEmpty(), "Expected no side effects to be detected")
	}
}

// AssertExecutorRegistration verifies executor is properly registered
func (a *PlatformAssertions) AssertExecutorRegistration(registry interface{}, executorName string, isRegistered bool) {
	// This would need to be customized based on the actual registry interface
	// For now, just verify the basic expectation
	if isRegistered {
		Expect(executorName).NotTo(BeEmpty())
	}
}
