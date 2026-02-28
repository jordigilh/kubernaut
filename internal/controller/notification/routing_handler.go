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
	kubernautnotif "github.com/jordigilh/kubernaut/pkg/notification"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
)

// ========================================
// ROUTING RESOLUTION
// ========================================

// resolveChannelsFromRoutingWithDetails resolves channels and returns routing details for BR-NOT-069 condition.
// Returns (channels, routingReason, routingMessage) so callers can set the correct RoutingResolved reason.
func (r *NotificationRequestReconciler) resolveChannelsFromRoutingWithDetails(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
) ([]notificationv1alpha1.Channel, string, string) {
	logger := log.FromContext(ctx)

	if r.Router == nil {
		logger.Info("No routing router initialized, using default console channel",
			"notification", notification.Name)
		return []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole},
			kubernautnotif.ReasonRoutingFallback,
			"No routing configuration, using console fallback"
	}

	routingAttrs := routing.RoutingAttributesFromSpec(notification)
	logger.V(1).Info("Routing attributes from spec", "attributes", routingAttrs)

	receiver := r.Router.FindReceiver(routingAttrs)
	channels := r.receiverToChannels(receiver)

	logger.Info("Resolved channels from routing",
		"notification", notification.Name,
		"receiver", receiver.Name,
		"channels", channels)

	isFallback := receiver.Name == "console-fallback" || len(channels) == 0
	var routingReason, routingMessage string
	if isFallback {
		routingReason = kubernautnotif.ReasonRoutingFallback
		attrsPart := r.formatAttributesForCondition(routingAttrs)
		routingMessage = fmt.Sprintf("No routing rules matched %s, using console fallback", attrsPart)
	} else {
		routingReason = kubernautnotif.ReasonRoutingRuleMatched
		attrsPart := r.formatAttributesForCondition(routingAttrs)
		channelsPart := r.formatChannelsForCondition(channels)
		routingMessage = fmt.Sprintf("Matched rule '%s' %s → channels: %s",
			receiver.Name, attrsPart, channelsPart)
	}

	return channels, routingReason, routingMessage
}

// ========================================
// FORMATTING HELPERS
// ========================================

// formatAttributesForCondition formats routing attributes for RoutingResolved condition message
func (r *NotificationRequestReconciler) formatAttributesForCondition(attrs map[string]string) string {
	if len(attrs) == 0 {
		return "(no attributes)"
	}

	pairs := make([]string, 0, len(attrs))
	for k, v := range attrs {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
	}
	return fmt.Sprintf("(attributes: %s)", strings.Join(pairs, ", "))
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
// BR-NOT-104: Returns receiver-qualified names for credential-bound channels (e.g., "slack:sre-critical").
func (r *NotificationRequestReconciler) receiverToChannels(receiver *routing.Receiver) []notificationv1alpha1.Channel {
	qualified := receiver.QualifiedChannels()

	if len(qualified) == 0 {
		return []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole}
	}

	channels := make([]notificationv1alpha1.Channel, len(qualified))
	for i, q := range qualified {
		channels[i] = notificationv1alpha1.Channel(q)
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
		Namespace: routing.GetConfigMapNamespace(),
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

	// Parse and validate structure first (before loading into router)
	newConfig, err := routing.ParseConfig(yamlData)
	if err != nil {
		return fmt.Errorf("failed to parse routing configuration: %w", err)
	}

	// BR-NOT-104: Validate credential refs before accepting the new config
	if err := newConfig.ValidateCredentialRefs(); err != nil {
		logger.Error(err, "New routing config has empty credentialRef, keeping previous config")
		return fmt.Errorf("credential validation failed, keeping previous config: %w", err)
	}

	// BR-NOT-104-003: Verify all credentialRefs resolve to actual files on disk
	if r.CredentialResolver != nil {
		var allRefs []string
		for _, recv := range newConfig.Receivers {
			for _, sc := range recv.SlackConfigs {
				if sc.CredentialRef != "" {
					allRefs = append(allRefs, sc.CredentialRef)
				}
			}
		}
		if len(allRefs) > 0 {
			if err := r.CredentialResolver.ValidateRefs(allRefs); err != nil {
				logger.Error(err, "Credential files missing for routing config, keeping previous config")
				return fmt.Errorf("credential file validation failed, keeping previous config: %w", err)
			}
		}
	}

	// BR-NOT-067: Routing table updated without restart
	if err := r.Router.LoadConfig(yamlData); err != nil {
		return fmt.Errorf("failed to load routing configuration: %w", err)
	}

	// BR-NOT-104: Rebuild per-receiver Slack delivery services
	r.rebuildSlackDeliveryServices(ctx, newConfig)

	logger.Info("Routing configuration loaded successfully from ConfigMap",
		"name", key.Name,
		"namespace", key.Namespace,
		"summary", r.Router.GetConfigSummary(),
	)

	return nil
}

// rebuildSlackDeliveryServices registers per-receiver Slack delivery services
// based on the current routing configuration and credential resolver.
// BR-NOT-104: Per-receiver delivery binding via receiver-qualified orchestrator keys.
func (r *NotificationRequestReconciler) rebuildSlackDeliveryServices(ctx context.Context, config *routing.Config) {
	logger := log.FromContext(ctx)

	if r.CredentialResolver == nil {
		logger.Info("Credential resolver not available, skipping Slack delivery registration")
		return
	}

	// Unregister stale Slack channels from previous config
	for _, key := range r.registeredSlackKeys {
		r.DeliveryOrchestrator.UnregisterChannel(key)
	}

	var newKeys []string
	for _, receiver := range config.Receivers {
		for i, sc := range receiver.SlackConfigs {
			webhookURL, err := r.CredentialResolver.Resolve(sc.CredentialRef)
			if err != nil {
				logger.Error(err, "Failed to resolve credential for Slack receiver",
					"receiver", receiver.Name,
					"credentialRef", sc.CredentialRef)
				continue
			}

			channelKey := fmt.Sprintf("slack:%s", receiver.Name)
			if len(receiver.SlackConfigs) > 1 {
				channelKey = fmt.Sprintf("slack:%s:%d", receiver.Name, i)
			}
			slackService := delivery.NewSlackDeliveryService(webhookURL)

			if r.CircuitBreaker != nil {
				cbSlack := delivery.NewCircuitBreakerSlackService(slackService, r.CircuitBreaker)
				r.DeliveryOrchestrator.RegisterChannel(channelKey, cbSlack)
			} else {
				r.DeliveryOrchestrator.RegisterChannel(channelKey, slackService)
			}

			newKeys = append(newKeys, channelKey)
			logger.Info("Registered per-receiver Slack delivery",
				"channelKey", channelKey,
				"receiver", receiver.Name,
				"credentialRef", sc.CredentialRef)
		}
	}
	r.registeredSlackKeys = newKeys
}


