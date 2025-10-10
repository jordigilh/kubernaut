/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package monitoring

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// EnhancedSideEffectDetector provides real side effect detection using Kubernetes events and AlertManager
type EnhancedSideEffectDetector struct {
	k8sClient   k8s.Client
	alertClient AlertClient
	log         *logrus.Logger
}

// NewEnhancedSideEffectDetector creates a new enhanced side effect detector
func NewEnhancedSideEffectDetector(k8sClient k8s.Client, alertClient AlertClient, log *logrus.Logger) *EnhancedSideEffectDetector {
	return &EnhancedSideEffectDetector{
		k8sClient:   k8sClient,
		alertClient: alertClient,
		log:         log,
	}
}

// DetectSideEffects analyzes if an action caused any negative side effects
func (d *EnhancedSideEffectDetector) DetectSideEffects(ctx context.Context, actionTrace *actionhistory.ResourceActionTrace) ([]SideEffect, error) {
	// Extract resource information from alert labels
	namespace := extractNamespaceFromLabels(actionTrace.AlertLabels)
	resourceName := extractResourceNameFromTrace(actionTrace)

	d.log.WithFields(logrus.Fields{
		"action_type":   actionTrace.ActionType,
		"action_id":     actionTrace.ActionID,
		"namespace":     namespace,
		"resource_name": resourceName,
	}).Debug("Detecting side effects via Kubernetes events and alerts")

	if actionTrace.ExecutionEndTime == nil {
		return nil, fmt.Errorf("action trace missing execution end time")
	}

	sideEffects := []SideEffect{}

	// 1. Check for new Kubernetes events that might indicate problems
	eventSideEffects, err := d.detectEventBasedSideEffects(ctx, actionTrace, namespace, resourceName)
	if err != nil {
		d.log.WithError(err).Warn("Failed to detect event-based side effects")
	} else {
		sideEffects = append(sideEffects, eventSideEffects...)
	}

	// 2. Check for new alerts that might have been triggered
	alertSideEffects, err := d.detectAlertBasedSideEffects(ctx, actionTrace, namespace)
	if err != nil {
		d.log.WithError(err).Warn("Failed to detect alert-based side effects")
	} else {
		sideEffects = append(sideEffects, alertSideEffects...)
	}

	// 3. Check for resource health degradation
	resourceSideEffects, err := d.detectResourceHealthDegradation(ctx, actionTrace, namespace, resourceName)
	if err != nil {
		d.log.WithError(err).Warn("Failed to detect resource health degradation")
	} else {
		sideEffects = append(sideEffects, resourceSideEffects...)
	}

	d.log.WithFields(logrus.Fields{
		"action_id":         actionTrace.ActionID,
		"side_effect_count": len(sideEffects),
	}).Info("Side effect detection completed")

	return sideEffects, nil
}

// CheckNewAlerts looks for new alerts that may have been triggered by the action
func (d *EnhancedSideEffectDetector) CheckNewAlerts(ctx context.Context, namespace string, since time.Time) ([]types.Alert, error) {
	d.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"since":     since,
	}).Debug("Checking for new alerts since action")

	if d.alertClient == nil {
		d.log.Warn("AlertClient not available for new alert detection")
		return []types.Alert{}, nil
	}

	// Get alert history since the action
	alertEvents, err := d.alertClient.GetAlertHistory(ctx, "", namespace, since, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to get alert history: %w", err)
	}

	var newAlerts []types.Alert
	for _, event := range alertEvents {
		// Only consider firing alerts as new problems
		if event.Status == "firing" && event.Timestamp.After(since) {
			alert := types.Alert{
				Name:        event.AlertName,
				Status:      event.Status,
				Severity:    event.Severity,
				Namespace:   event.Namespace,
				Labels:      event.Labels,
				Annotations: event.Annotations,
			}
			newAlerts = append(newAlerts, alert)
		}
	}

	d.log.WithFields(logrus.Fields{
		"namespace":       namespace,
		"new_alert_count": len(newAlerts),
	}).Debug("New alert detection completed")

	return newAlerts, nil
}

// detectEventBasedSideEffects checks Kubernetes events for signs of problems
func (d *EnhancedSideEffectDetector) detectEventBasedSideEffects(ctx context.Context, actionTrace *actionhistory.ResourceActionTrace, namespace, resourceName string) ([]SideEffect, error) {
	var sideEffects []SideEffect

	// Get events from the namespace since action execution
	events, err := d.k8sClient.GetEvents(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	executionEnd := *actionTrace.ExecutionEndTime

	// Look for problematic events after action execution
	for _, event := range events.Items {
		// Only consider events after our action
		if event.CreationTimestamp.Time.Before(executionEnd) {
			continue
		}

		// Check if this event indicates a problem
		if d.isProblematicEvent(&event, resourceName) {
			severity := d.categorizeEventSeverity(&event)
			sideEffect := SideEffect{
				Type:        "kubernetes_event",
				Severity:    severity,
				Description: fmt.Sprintf("Problematic event detected: %s - %s", event.Reason, event.Message),
				Evidence: map[string]interface{}{
					"event_type":      event.Type,
					"event_reason":    event.Reason,
					"event_message":   event.Message,
					"involved_object": event.InvolvedObject,
					"source":          event.Source,
				},
				DetectedAt: event.CreationTimestamp.Time,
			}
			sideEffects = append(sideEffects, sideEffect)
		}
	}

	return sideEffects, nil
}

// detectAlertBasedSideEffects checks for new alerts triggered after the action
func (d *EnhancedSideEffectDetector) detectAlertBasedSideEffects(ctx context.Context, actionTrace *actionhistory.ResourceActionTrace, namespace string) ([]SideEffect, error) {
	var sideEffects []SideEffect

	if d.alertClient == nil {
		return sideEffects, nil
	}

	executionEnd := *actionTrace.ExecutionEndTime

	// Check for new alerts in the same namespace
	newAlerts, err := d.CheckNewAlerts(ctx, namespace, executionEnd)
	if err != nil {
		return nil, err
	}

	for _, alert := range newAlerts {
		// Skip if this is the same alert that triggered our action
		if alert.Name == actionTrace.AlertName {
			continue
		}

		// Check if this alert might be related to our action
		if d.isAlertRelatedToAction(&alert, namespace, extractResourceNameFromTrace(actionTrace)) {
			severity := d.categorizeAlertSeverity(alert.Severity)
			sideEffect := SideEffect{
				Type:        "new_alert",
				Severity:    severity,
				Description: fmt.Sprintf("New alert triggered: %s", alert.Name),
				Evidence: map[string]interface{}{
					"alert_name":        alert.Name,
					"alert_labels":      alert.Labels,
					"alert_annotations": alert.Annotations,
					"severity":          alert.Severity,
				},
				DetectedAt: time.Now(), // We don't have exact firing time from our simple implementation
			}
			sideEffects = append(sideEffects, sideEffect)
		}
	}

	return sideEffects, nil
}

// detectResourceHealthDegradation checks if the action caused resource health issues
func (d *EnhancedSideEffectDetector) detectResourceHealthDegradation(ctx context.Context, actionTrace *actionhistory.ResourceActionTrace, namespace, resourceName string) ([]SideEffect, error) {
	var sideEffects []SideEffect

	// Check resource health based on action type
	switch actionTrace.ActionType {
	case "scale_deployment":
		degradation, err := d.checkDeploymentHealthDegradation(ctx, namespace, resourceName)
		if err != nil {
			return nil, err
		}
		sideEffects = append(sideEffects, degradation...)

	case "restart_pod":
		degradation, err := d.checkPodHealthDegradation(ctx, namespace, resourceName)
		if err != nil {
			return nil, err
		}
		sideEffects = append(sideEffects, degradation...)

	case "increase_resources":
		degradation, err := d.checkResourceConstraintIssues(ctx, namespace)
		if err != nil {
			return nil, err
		}
		sideEffects = append(sideEffects, degradation...)
	}

	return sideEffects, nil
}

// checkDeploymentHealthDegradation checks if scaling caused deployment health issues
func (d *EnhancedSideEffectDetector) checkDeploymentHealthDegradation(ctx context.Context, namespace, resourceName string) ([]SideEffect, error) {
	var sideEffects []SideEffect

	deployment, err := d.k8sClient.GetDeployment(ctx, namespace, resourceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	// Check for deployment health issues
	if deployment.Status.ReadyReplicas < deployment.Status.Replicas {
		unreadyReplicas := deployment.Status.Replicas - deployment.Status.ReadyReplicas
		severity := "medium"
		if float64(unreadyReplicas)/float64(deployment.Status.Replicas) > 0.5 {
			severity = "high"
		}

		sideEffect := SideEffect{
			Type:        "resource_issue",
			Severity:    severity,
			Description: fmt.Sprintf("Deployment has %d unready replicas after scaling", unreadyReplicas),
			Evidence: map[string]interface{}{
				"deployment_name":  deployment.Name,
				"desired_replicas": deployment.Status.Replicas,
				"ready_replicas":   deployment.Status.ReadyReplicas,
				"unready_replicas": int(unreadyReplicas),
			},
			DetectedAt: time.Now(),
		}
		sideEffects = append(sideEffects, sideEffect)
	}

	return sideEffects, nil
}

// checkPodHealthDegradation checks if pod restart caused health issues
func (d *EnhancedSideEffectDetector) checkPodHealthDegradation(ctx context.Context, namespace, resourceName string) ([]SideEffect, error) {
	var sideEffects []SideEffect

	// List pods to check for restart issues
	pods, err := d.k8sClient.ListPodsWithLabel(ctx, namespace, fmt.Sprintf("app=%s", resourceName))
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	for _, pod := range pods.Items {
		// Check for excessive restart counts
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.RestartCount > 3 {
				sideEffect := SideEffect{
					Type:        "resource_issue",
					Severity:    "medium",
					Description: fmt.Sprintf("Container %s has high restart count (%d)", containerStatus.Name, containerStatus.RestartCount),
					Evidence: map[string]interface{}{
						"pod_name":        pod.Name,
						"container_name":  containerStatus.Name,
						"restart_count":   containerStatus.RestartCount,
						"container_ready": containerStatus.Ready,
					},
					DetectedAt: time.Now(),
				}
				sideEffects = append(sideEffects, sideEffect)
			}
		}
	}

	return sideEffects, nil
}

// checkResourceConstraintIssues checks if resource increases caused constraint issues
func (d *EnhancedSideEffectDetector) checkResourceConstraintIssues(ctx context.Context, namespace string) ([]SideEffect, error) {
	var sideEffects []SideEffect

	// Check if resource quotas were exceeded
	quotas, err := d.k8sClient.GetResourceQuotas(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource quotas: %w", err)
	}

	for _, quota := range quotas.Items {
		for resourceName, used := range quota.Status.Used {
			if hard, exists := quota.Status.Hard[resourceName]; exists {
				usedQuantity := used.MilliValue()
				hardQuantity := hard.MilliValue()

				// Check if we're close to or exceeding limits (>90%)
				if usedQuantity > (hardQuantity * 90 / 100) {
					severity := "medium"
					if usedQuantity >= hardQuantity {
						severity = "high"
					}

					sideEffect := SideEffect{
						Type:        "resource_constraint",
						Severity:    severity,
						Description: fmt.Sprintf("Resource quota %s is %d%% utilized", resourceName, (usedQuantity*100)/hardQuantity),
						Evidence: map[string]interface{}{
							"quota_name":      quota.Name,
							"resource_name":   resourceName,
							"used":            used.String(),
							"hard_limit":      hard.String(),
							"utilization_pct": (usedQuantity * 100) / hardQuantity,
						},
						DetectedAt: time.Now(),
					}
					sideEffects = append(sideEffects, sideEffect)
				}
			}
		}
	}

	return sideEffects, nil
}

// isProblematicEvent determines if a Kubernetes event indicates a problem
func (d *EnhancedSideEffectDetector) isProblematicEvent(event *corev1.Event, resourceName string) bool {
	// Check event type
	if event.Type != "Warning" && event.Type != "Error" {
		return false
	}

	// Check if event is related to our resource
	if !d.isEventRelatedToResource(event, resourceName) {
		return false
	}

	// List of problematic event reasons
	problematicReasons := []string{
		"Failed", "FailedScheduling", "FailedMount", "FailedCreate",
		"Unhealthy", "BackOff", "ErrImagePull", "ImagePullBackOff",
		"FailedSync", "FailedValidation", "FailedPostStartHook",
		"FailedPreStopHook", "ProbeWarning", "DNSConfigForming",
		"InspectFailed", "NetworkNotReady", "Rebooted",
	}

	for _, reason := range problematicReasons {
		if strings.Contains(event.Reason, reason) {
			return true
		}
	}

	return false
}

// isEventRelatedToResource checks if an event is related to the resource that was acted upon
func (d *EnhancedSideEffectDetector) isEventRelatedToResource(event *corev1.Event, resourceName string) bool {
	// Check if event involves the same resource
	if event.InvolvedObject.Name == resourceName {
		return true
	}

	// Check if event involves related resources (e.g., pods created by deployment)
	if strings.Contains(event.InvolvedObject.Name, resourceName) {
		return true
	}

	return false
}

// isAlertRelatedToAction determines if an alert might be related to our action
func (d *EnhancedSideEffectDetector) isAlertRelatedToAction(alert *types.Alert, namespace, resourceName string) bool {
	// Check if alert is from the same namespace
	if alert.Namespace != namespace {
		return false
	}

	// Check if alert mentions the same resource
	if labels, exists := alert.Labels["deployment"]; exists && labels == resourceName {
		return true
	}
	if labels, exists := alert.Labels["pod"]; exists && strings.Contains(labels, resourceName) {
		return true
	}

	// Check alert name for resource correlation
	if strings.Contains(alert.Name, resourceName) {
		return true
	}

	return false
}

// categorizeEventSeverity determines the severity of a Kubernetes event
func (d *EnhancedSideEffectDetector) categorizeEventSeverity(event *corev1.Event) string {
	if event.Type == "Error" {
		return "high"
	}

	criticalReasons := []string{"Failed", "FailedScheduling", "ErrImagePull", "ImagePullBackOff"}
	for _, reason := range criticalReasons {
		if strings.Contains(event.Reason, reason) {
			return "high"
		}
	}

	return "medium"
}

// categorizeAlertSeverity maps alert severity to side effect severity
func (d *EnhancedSideEffectDetector) categorizeAlertSeverity(alertSeverity string) string {
	switch strings.ToLower(alertSeverity) {
	case "critical":
		return "critical"
	case "warning":
		return "medium"
	case "info":
		return "low"
	default:
		return "medium"
	}
}

// extractNamespaceFromLabels extracts namespace from alert labels
func extractNamespaceFromLabels(labels actionhistory.JSONData) string {
	if labels == nil {
		return "default"
	}

	labelsMap := map[string]interface{}(labels)

	if ns, ok := labelsMap["namespace"]; ok {
		if nsStr, ok := ns.(string); ok {
			return nsStr
		}
	}

	return "default"
}

// extractResourceNameFromTrace extracts resource name from action trace parameters or alert labels
func extractResourceNameFromTrace(trace *actionhistory.ResourceActionTrace) string {
	// First try to get from action parameters
	if trace.ActionParameters != nil {
		paramsMap := map[string]interface{}(trace.ActionParameters)

		// Check common parameter names
		if name, ok := paramsMap["deployment"]; ok {
			if nameStr, ok := name.(string); ok {
				return nameStr
			}
		}
		if name, ok := paramsMap["resource"]; ok {
			if nameStr, ok := name.(string); ok {
				return nameStr
			}
		}
		if name, ok := paramsMap["name"]; ok {
			if nameStr, ok := name.(string); ok {
				return nameStr
			}
		}
	}

	// Try to get from alert labels
	if trace.AlertLabels != nil {
		labelsMap := map[string]interface{}(trace.AlertLabels)

		// Check common label names
		if name, ok := labelsMap["deployment"]; ok {
			if nameStr, ok := name.(string); ok {
				return nameStr
			}
		}
		if name, ok := labelsMap["pod"]; ok {
			if nameStr, ok := name.(string); ok {
				// Extract deployment name from pod name (e.g., "webapp-123" -> "webapp")
				parts := strings.Split(nameStr, "-")
				if len(parts) > 1 {
					return strings.Join(parts[:len(parts)-1], "-")
				}
				return nameStr
			}
		}
		if name, ok := labelsMap["service"]; ok {
			if nameStr, ok := name.(string); ok {
				return nameStr
			}
		}
		if name, ok := labelsMap["app"]; ok {
			if nameStr, ok := name.(string); ok {
				return nameStr
			}
		}
	}

	// Fallback to alert name if no resource name found
	return trace.AlertName
}
