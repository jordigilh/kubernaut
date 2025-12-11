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

// ResolveChannelsForNotification resolves the delivery channels for a notification
// based on its labels and the routing configuration.
//
// BR-NOT-065: Channel Routing Based on Labels
//
// This function implements the label-based routing logic:
//  1. Extract labels from notification.ObjectMeta.Labels
//  2. Match labels against routing rules (first match wins)
//  3. Return the channels configured for the matched receiver
//
// If config is nil, returns ["console"] as fallback.
// If no routes match, returns channels from the default receiver.
func ResolveChannelsForNotification(config *Config, notification *notificationv1alpha1.NotificationRequest) []string {
	if config == nil {
		// Use default config with console fallback
		config = DefaultConfig()
	}

	if notification == nil {
		return []string{"console"}
	}

	// Extract labels from notification
	labels := notification.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	// Find matching receiver using routing rules
	receiverName := config.Route.FindReceiver(labels)

	// Get receiver configuration
	receiver := config.GetReceiver(receiverName)
	if receiver == nil {
		// Fallback to console if receiver not found
		return []string{"console"}
	}

	// Return channels configured for this receiver
	return receiver.GetChannels()
}

// ShouldUseRoutingRules determines if routing rules should be used
// for the given notification.
//
// BR-NOT-065: If spec.channels is NOT specified, use routing rules.
// If spec.channels IS specified, use those channels directly.
func ShouldUseRoutingRules(notification *notificationv1alpha1.NotificationRequest) bool {
	if notification == nil {
		return true
	}
	return len(notification.Spec.Channels) == 0
}

// GetEffectiveChannels returns the channels to use for delivery.
//
// BR-NOT-065: Channel resolution priority:
//  1. If spec.channels is specified, use those channels
//  2. Otherwise, resolve channels from labels using routing rules
func GetEffectiveChannels(config *Config, notification *notificationv1alpha1.NotificationRequest) []notificationv1alpha1.Channel {
	if notification == nil {
		return []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole}
	}

	// If channels explicitly specified, use them
	if len(notification.Spec.Channels) > 0 {
		return notification.Spec.Channels
	}

	// Resolve from routing rules
	channelStrings := ResolveChannelsForNotification(config, notification)

	// Convert string channels to typed channels
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
		}
	}

	// Ensure at least console as fallback
	if len(channels) == 0 {
		channels = append(channels, notificationv1alpha1.ChannelConsole)
	}

	return channels
}




