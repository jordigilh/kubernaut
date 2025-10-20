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

package formatting

import (
	"fmt"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// ConsoleFormatter formats notifications for console output
type ConsoleFormatter struct{}

// NewConsoleFormatter creates a new console formatter
func NewConsoleFormatter() *ConsoleFormatter {
	return &ConsoleFormatter{}
}

// Format formats a notification for console output
// Returns plain text formatted string
func (f *ConsoleFormatter) Format(notification *notificationv1alpha1.NotificationRequest) (string, error) {
	// Simple text format for console
	// Priority emoji map
	priorityEmoji := map[notificationv1alpha1.NotificationPriority]string{
		notificationv1alpha1.NotificationPriorityCritical: "🚨",
		notificationv1alpha1.NotificationPriorityHigh:     "⚠️",
		notificationv1alpha1.NotificationPriorityMedium:   "ℹ️",
		notificationv1alpha1.NotificationPriorityLow:      "💬",
	}

	emoji := priorityEmoji[notification.Spec.Priority]
	if emoji == "" {
		emoji = "📢"
	}

	formatted := fmt.Sprintf(
		"%s [%s] %s\n%s\n",
		emoji,
		notification.Spec.Priority,
		notification.Spec.Subject,
		notification.Spec.Body,
	)

	return formatted, nil
}
