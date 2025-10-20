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
	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// SlackFormatter formats notifications for Slack Block Kit
type SlackFormatter struct{}

// NewSlackFormatter creates a new Slack formatter
func NewSlackFormatter() *SlackFormatter {
	return &SlackFormatter{}
}

// Format formats a notification for Slack Block Kit
// Returns a map representing Block Kit JSON structure
// Reference: https://api.slack.com/block-kit
func (f *SlackFormatter) Format(notification *notificationv1alpha1.NotificationRequest) (interface{}, error) {
	// TODO: Implement Slack Block Kit formatting (Day 3)
	// - Header block with priority emoji + subject
	// - Section block with body text
	// - Context block with metadata
	// - Action links (if provided in notification spec)
	// - Enforce 40KB limit
	return nil, nil
}
