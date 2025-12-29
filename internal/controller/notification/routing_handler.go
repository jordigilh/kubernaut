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

// ========================================
// ROUTING HANDLER (Pattern 4: Controller Decomposition)
// 📋 Pattern: Pattern 4 - Controller Decomposition
// See: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §5
// ========================================
//
// This file contains notification routing logic extracted from the main controller
// to improve maintainability and testability per Pattern 4.
//
// BENEFITS:
// - ~250 lines extracted from main controller
// - Routing logic isolated and maintainable
// - Clear separation of concerns
// - Easy to test routing rules independently
//
// RESPONSIBILITIES:
// - Channel resolution from routing rules (BR-NOT-065)
// - ConfigMap watch and hot-reload (BR-NOT-067)
// - Routing condition formatting (BR-NOT-069)
// - Receiver-to-channel mapping
//
// BR REFERENCES:
// - BR-NOT-065: Use routing rules to determine channels
// - BR-NOT-067: Hot-reload routing configuration from ConfigMap
// - BR-NOT-069: Routing condition visibility
// ========================================

package notification

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
)

// ========================================
// ROUTING RESOLUTION
// ========================================

// resolveChannelsFromRouting resolves notification channels using routing rules.
// BR-NOT-065: Use routing rules to determine channels based on CRD labels.
//
// Routing Priority (DD-WE-004):
//   - PreviousExecutionFailed → CRITICAL (PagerDuty)
//   - ExhaustedRetries → HIGH (Slack)
//   - ResourceBusy/RecentlyRemediated → LOW (Console bulk)
func (r *NotificationRequestReconciler) resolveChannelsFromRouting(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
) []notificationv1alpha1.Channel {
	channels, _ := r.resolveChannelsFromRoutingWithDetails(ctx, notification)
	return channels
}

// resolveChannelsFromRoutingWithDetails resolves channels and returns routing details for BR-NOT-069 condition.
// BR-NOT-069: Return routing message for RoutingResolved condition visibility
func (r *NotificationRequestReconciler) resolveChannelsFromRoutingWithDetails(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
) ([]notificationv1alpha1.Channel, string) {
	logger := log.FromContext(ctx)

	// BR-NOT-067: Use Router for thread-safe routing with hot-reload support
	if r.Router == nil {
		logger.Info("No routing router initialized, using default console channel",
			"notification", notification.Name)
		return []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole},
			"No routing configuration, using console fallback"
	}

	// Find receiver based on notification labels
	labels := notification.Labels
	if labels == nil {
		labels = make(map[string]string)
	}

	// Extract kubernaut.ai labels for routing
	routingLabels := make(map[string]string)
	for k, v := range labels {
		if strings.HasPrefix(k, "kubernaut.ai/") {
			routingLabels[k] = v
		}
	}
	logger.V(1).Info("Routing labels", "labels", routingLabels)

	// BR-NOT-067: Find matching receiver using thread-safe Router
	receiver := r.Router.FindReceiver(labels)

	// Convert receiver to channels
	channels := r.receiverToChannels(receiver)

	logger.Info("Resolved channels from routing",
		"notification", notification.Name,
		"receiver", receiver.Name,
		"channels", channels)

	// BR-NOT-069: Build routing message for RoutingResolved condition
	var routingMessage string
	if receiver.Name == "console-fallback" || len(channels) == 0 {
		// Fallback scenario
		labelsPart := r.formatLabelsForCondition(routingLabels)
		routingMessage = fmt.Sprintf("No routing rules matched %s, using console fallback", labelsPart)
	} else {
		// Rule matched
		labelsPart := r.formatLabelsForCondition(routingLabels)
		channelsPart := r.formatChannelsForCondition(channels)
		routingMessage = fmt.Sprintf("Matched rule '%s' %s → channels: %s",
			receiver.Name, labelsPart, channelsPart)
	}

	return channels, routingMessage
}

// ========================================
// FORMATTING HELPERS
// ========================================

// formatLabelsForCondition formats labels for RoutingResolved condition message
func (r *NotificationRequestReconciler) formatLabelsForCondition(labels map[string]string) string {
	if len(labels) == 0 {
		return "(no labels)"
	}

	// Format as (key1=value1, key2=value2)
	labelPairs := make([]string, 0, len(labels))
	for k, v := range labels {
		// Strip kubernaut.ai/ prefix for readability
		shortKey := strings.TrimPrefix(k, "kubernaut.ai/")
		labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", shortKey, v))
	}
	return fmt.Sprintf("(labels: %s)", strings.Join(labelPairs, ", "))
}

// formatChannelsForCondition formats channels for RoutingResolved condition message
func (r *NotificationRequestReconciler) formatChannelsForCondition(channels []notificationv1alpha1.Channel) string {
	if len(channels) == 0 {
		return "none"
	}

	channelNames := make([]string, len(channels))
	for i, ch := range channels {
		channelNames[i] = string(ch)
	}
	return strings.Join(channelNames, ", ")
}

// ========================================
// RECEIVER MAPPING
// ========================================

// receiverToChannels converts a routing.Receiver to a list of notification channels.
func (r *NotificationRequestReconciler) receiverToChannels(receiver *routing.Receiver) []notificationv1alpha1.Channel {
	var channels []notificationv1alpha1.Channel

	// Map receiver configs to CRD channel types
	if len(receiver.SlackConfigs) > 0 {
		channels = append(channels, notificationv1alpha1.ChannelSlack)
	}
	if len(receiver.PagerDutyConfigs) > 0 {
		// PagerDuty uses webhook channel type
		channels = append(channels, notificationv1alpha1.ChannelWebhook)
	}
	if len(receiver.EmailConfigs) > 0 {
		channels = append(channels, notificationv1alpha1.ChannelEmail)
	}
	if len(receiver.WebhookConfigs) > 0 {
		channels = append(channels, notificationv1alpha1.ChannelWebhook)
	}
	if len(receiver.ConsoleConfigs) > 0 {
		channels = append(channels, notificationv1alpha1.ChannelConsole)
	}

	// Default to console if no channels configured
	if len(channels) == 0 {
		channels = append(channels, notificationv1alpha1.ChannelConsole)
	}

	return channels
}

// ========================================
// CONFIGMAP WATCH & HOT-RELOAD
// ========================================

// handleConfigMapChange handles changes to the routing ConfigMap.
// It reloads the routing configuration when the ConfigMap is created, updated, or deleted.
func (r *NotificationRequestReconciler) handleConfigMapChange(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	// Verify this is the routing ConfigMap
	if !routing.IsRoutingConfigMap(obj.GetName(), obj.GetNamespace()) {
		return nil
	}

	logger.Info("Routing ConfigMap changed, reloading configuration",
		"name", obj.GetName(),
		"namespace", obj.GetNamespace(),
	)

	// Reload routing configuration
	if err := r.loadRoutingConfigFromCluster(ctx); err != nil {
		logger.Error(err, "Failed to reload routing configuration, keeping previous config")
	}

	// Return empty - we don't need to reconcile any specific NotificationRequest
	// The new config will be used for future notifications
	return nil
}

// loadRoutingConfigFromCluster loads routing configuration from the cluster ConfigMap.
// BR-NOT-067: ConfigMap changes detected within 30 seconds
func (r *NotificationRequestReconciler) loadRoutingConfigFromCluster(ctx context.Context) error {
	logger := log.FromContext(ctx)

	// Fetch the routing ConfigMap
	configMap := &corev1.ConfigMap{}
	key := types.NamespacedName{
		Name:      routing.DefaultConfigMapName,
		Namespace: routing.DefaultConfigMapNamespace,
	}

	if err := r.Get(ctx, key, configMap); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Routing ConfigMap not found, using default configuration",
				"name", key.Name,
				"namespace", key.Namespace,
			)
			// Use default config (already set in NewRouter)
			return nil
		}
		return fmt.Errorf("failed to get routing ConfigMap: %w", err)
	}

	// Extract routing YAML from ConfigMap
	yamlData, ok := routing.ExtractRoutingConfig(configMap.Data)
	if !ok {
		logger.Info("Routing ConfigMap found but missing routing.yaml key, using default configuration",
			"name", key.Name,
			"namespace", key.Namespace,
		)
		return nil
	}

	// Load the new configuration
	// BR-NOT-067: Routing table updated without restart
	if err := r.Router.LoadConfig(yamlData); err != nil {
		return fmt.Errorf("failed to load routing configuration: %w", err)
	}

	logger.Info("Routing configuration loaded successfully from ConfigMap",
		"name", key.Name,
		"namespace", key.Namespace,
		"summary", r.Router.GetConfigSummary(),
	)

	return nil
}


