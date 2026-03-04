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

package routing

import (
	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// RoutingAttributesFromSpec builds a routing attributes map from a NotificationRequest's
// spec fields and metadata, replacing the previous label-based approach.
//
// Issue #91: Routing now uses immutable spec fields instead of mutable labels.
//
// Attribute sources:
//   - Top-level spec fields: type, severity, phase, reviewSource, priority
//   - spec.metadata entries: skip-reason, investigation-outcome, environment, etc.
func RoutingAttributesFromSpec(notification *notificationv1alpha1.NotificationRequest) map[string]string {
	attrs := make(map[string]string)
	if notification == nil {
		return attrs
	}

	spec := &notification.Spec
	if spec.Type != "" {
		attrs[AttrType] = string(spec.Type)
	}
	if spec.Severity != "" {
		attrs[AttrSeverity] = spec.Severity
	}
	if spec.Phase != "" {
		attrs[AttrPhase] = spec.Phase
	}
	if spec.ReviewSource != "" {
		attrs[AttrReviewSource] = spec.ReviewSource
	}
	if spec.Priority != "" {
		attrs[AttrPriority] = string(spec.Priority)
	}

	for k, v := range spec.Metadata {
		if _, exists := attrs[k]; !exists {
			attrs[k] = v
		}
	}

	return attrs
}

// ResolveChannelsForNotification resolves the delivery channels for a notification
// based on its spec fields and the routing configuration.
//
// BR-NOT-065: Channel Routing Based on Spec Fields
//
// This function implements the spec-field-based routing logic:
//  1. Extract routing attributes from spec fields + metadata
//  2. Match attributes against routing rules (first match wins)
//  3. Return the channels configured for the matched receiver
//
// If config is nil, returns ["console"] as fallback.
// If no routes match, returns channels from the default receiver.
func ResolveChannelsForNotification(config *Config, notification *notificationv1alpha1.NotificationRequest) []string {
	if config == nil {
		config = DefaultConfig()
	}

	if notification == nil {
		return []string{"console"}
	}

	attrs := RoutingAttributesFromSpec(notification)

	receiverName := config.Route.FindReceiver(attrs)

	receiver := config.GetReceiver(receiverName)
	if receiver == nil {
		return []string{"console"}
	}

	return receiver.GetChannels()
}

// ShouldUseRoutingRules always returns true since Spec.Channels was removed (#261).
// Kept for backward compatibility with any callers; will be removed post-v1.0.
func ShouldUseRoutingRules(notification *notificationv1alpha1.NotificationRequest) bool {
	return true
}

// GetEffectiveChannels resolves channels from routing rules.
// Since Spec.Channels was removed (#261), routing is always used.
func GetEffectiveChannels(config *Config, notification *notificationv1alpha1.NotificationRequest) []notificationv1alpha1.Channel {
	if notification == nil {
		return []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole}
	}

	channelStrings := ResolveChannelsForNotification(config, notification)
	return convertToTypedChannels(channelStrings)
}

// convertToTypedChannels converts string channel names to typed Channel values.
func convertToTypedChannels(channelStrings []string) []notificationv1alpha1.Channel {
	var channels []notificationv1alpha1.Channel

	for _, ch := range channelStrings {
		switch ch {
		case "slack":
			channels = append(channels, notificationv1alpha1.ChannelSlack)
		case "email":
			channels = append(channels, notificationv1alpha1.ChannelEmail)
		case "pagerduty":
			// PagerDuty is delivered via webhook
			channels = append(channels, notificationv1alpha1.ChannelWebhook)
		case "webhook":
			channels = append(channels, notificationv1alpha1.ChannelWebhook)
		case "teams":
			channels = append(channels, notificationv1alpha1.ChannelTeams)
		case "sms":
			channels = append(channels, notificationv1alpha1.ChannelSMS)
		case "console":
			channels = append(channels, notificationv1alpha1.ChannelConsole)
		case "file":
			channels = append(channels, notificationv1alpha1.ChannelFile)
		case "log":
			channels = append(channels, notificationv1alpha1.ChannelLog)
		}
	}

	// Ensure at least console as fallback
	if len(channels) == 0 {
		channels = append(channels, notificationv1alpha1.ChannelConsole)
	}

	return channels
}
