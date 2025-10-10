<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package notifications

import (
	"time"

	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// notificationBuilder implements the NotificationBuilder interface
type notificationBuilder struct {
	notification Notification
}

// NewNotificationBuilder creates a new notification builder
func NewNotificationBuilder() NotificationBuilder {
	return &notificationBuilder{
		notification: Notification{
			ID:        uuid.New().String(),
			Level:     NotificationLevelInfo,
			Timestamp: time.Now(),
			Metadata:  make(map[string]string),
			Tags:      make([]string, 0),
		},
	}
}

func (nb *notificationBuilder) WithLevel(level NotificationLevel) NotificationBuilder {
	nb.notification.Level = level
	return nb
}

func (nb *notificationBuilder) WithTitle(title string) NotificationBuilder {
	nb.notification.Title = title
	return nb
}

func (nb *notificationBuilder) WithMessage(message string) NotificationBuilder {
	nb.notification.Message = message
	return nb
}

func (nb *notificationBuilder) WithSource(source string) NotificationBuilder {
	nb.notification.Source = source
	return nb
}

func (nb *notificationBuilder) WithComponent(component string) NotificationBuilder {
	nb.notification.Component = component
	return nb
}

func (nb *notificationBuilder) WithAlert(alert types.Alert) NotificationBuilder {
	nb.notification.AlertName = alert.Name
	nb.notification.Namespace = alert.Namespace
	nb.notification.Resource = alert.Resource

	// Add alert metadata
	if nb.notification.Metadata == nil {
		nb.notification.Metadata = make(map[string]string)
	}

	nb.notification.Metadata["alert_severity"] = alert.Severity
	nb.notification.Metadata["alert_status"] = alert.Status

	if alert.Description != "" {
		nb.notification.Metadata["alert_description"] = alert.Description
	}

	// Add alert labels as metadata
	for key, value := range alert.Labels {
		nb.notification.Metadata["alert_label_"+key] = value
	}

	// Add alert annotations as metadata
	for key, value := range alert.Annotations {
		nb.notification.Metadata["alert_annotation_"+key] = value
	}

	return nb
}

func (nb *notificationBuilder) WithAction(action string) NotificationBuilder {
	nb.notification.Action = action
	return nb
}

func (nb *notificationBuilder) WithMetadata(key, value string) NotificationBuilder {
	if nb.notification.Metadata == nil {
		nb.notification.Metadata = make(map[string]string)
	}
	nb.notification.Metadata[key] = value
	return nb
}

func (nb *notificationBuilder) WithTag(tag string) NotificationBuilder {
	if nb.notification.Tags == nil {
		nb.notification.Tags = make([]string, 0)
	}
	nb.notification.Tags = append(nb.notification.Tags, tag)
	return nb
}

func (nb *notificationBuilder) WithTags(tags ...string) NotificationBuilder {
	if nb.notification.Tags == nil {
		nb.notification.Tags = make([]string, 0)
	}
	nb.notification.Tags = append(nb.notification.Tags, tags...)
	return nb
}

func (nb *notificationBuilder) Build() Notification {
	return nb.notification
}

// Ensure interface compliance
var _ NotificationBuilder = (*notificationBuilder)(nil)
