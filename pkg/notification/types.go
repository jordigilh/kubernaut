/*
Copyright 2025 Kubernaut.

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

package notification

import (
	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// Re-export notification API types for convenience
// This allows users to import "github.com/jordigilh/kubernaut/pkg/notification"
// instead of the longer "github.com/jordigilh/kubernaut/api/notification/v1alpha1"

type (
	// NotificationRequest is the main CRD type
	NotificationRequest = notificationv1alpha1.NotificationRequest

	// NotificationRequestSpec defines the desired state
	NotificationRequestSpec = notificationv1alpha1.NotificationRequestSpec

	// NotificationRequestStatus defines the observed state
	NotificationRequestStatus = notificationv1alpha1.NotificationRequestStatus

	// NotificationRequestList contains a list of NotificationRequest
	NotificationRequestList = notificationv1alpha1.NotificationRequestList

	// NotificationType defines the type of notification
	NotificationType = notificationv1alpha1.NotificationType

	// NotificationPriority defines the priority level
	NotificationPriority = notificationv1alpha1.NotificationPriority

	// NotificationPhase defines the lifecycle phase
	NotificationPhase = notificationv1alpha1.NotificationPhase

	// Channel defines delivery channels
	Channel = notificationv1alpha1.Channel

	// Recipient represents a notification recipient
	Recipient = notificationv1alpha1.Recipient

	// RetryPolicy defines retry behavior
	RetryPolicy = notificationv1alpha1.RetryPolicy

	// ActionLink represents external service links
	ActionLink = notificationv1alpha1.ActionLink
)

// Re-export notification type constants
const (
	NotificationTypeEscalation   = notificationv1alpha1.NotificationTypeEscalation
	NotificationTypeSimple       = notificationv1alpha1.NotificationTypeSimple
	NotificationTypeStatusUpdate = notificationv1alpha1.NotificationTypeStatusUpdate
)

// Re-export priority constants
const (
	NotificationPriorityCritical = notificationv1alpha1.NotificationPriorityCritical
	NotificationPriorityHigh     = notificationv1alpha1.NotificationPriorityHigh
	NotificationPriorityMedium   = notificationv1alpha1.NotificationPriorityMedium
	NotificationPriorityLow      = notificationv1alpha1.NotificationPriorityLow
)

// Re-export phase constants
const (
	NotificationPhasePending       = notificationv1alpha1.NotificationPhasePending
	NotificationPhaseSending       = notificationv1alpha1.NotificationPhaseSending
	NotificationPhaseSent          = notificationv1alpha1.NotificationPhaseSent
	NotificationPhasePartiallySent = notificationv1alpha1.NotificationPhasePartiallySent
	NotificationPhaseFailed        = notificationv1alpha1.NotificationPhaseFailed
)

// Re-export channel constants
const (
	ChannelEmail   = notificationv1alpha1.ChannelEmail
	ChannelSlack   = notificationv1alpha1.ChannelSlack
	ChannelTeams   = notificationv1alpha1.ChannelTeams
	ChannelSMS     = notificationv1alpha1.ChannelSMS
	ChannelWebhook = notificationv1alpha1.ChannelWebhook
	ChannelConsole = notificationv1alpha1.ChannelConsole
)
