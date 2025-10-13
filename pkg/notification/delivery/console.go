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

package delivery

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// ConsoleDeliveryService delivers notifications to console (stdout)
type ConsoleDeliveryService struct {
	logger *logrus.Logger
}

// NewConsoleDeliveryService creates a new console delivery service
func NewConsoleDeliveryService(logger *logrus.Logger) *ConsoleDeliveryService {
	if logger == nil {
		logger = logrus.New()
	}
	return &ConsoleDeliveryService{
		logger: logger,
	}
}

// Deliver delivers a notification to console (stdout)
// BR-NOT-053: At-least-once delivery (console always succeeds)
func (s *ConsoleDeliveryService) Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	// Log structured notification data
	s.logger.WithFields(logrus.Fields{
		"notification": notification.Name,
		"namespace":    notification.Namespace,
		"type":         notification.Spec.Type,
		"priority":     notification.Spec.Priority,
		"subject":      notification.Spec.Subject,
		"timestamp":    time.Now().Format(time.RFC3339),
	}).Info("Notification delivered to console")

	// Format for console output
	formattedMessage := fmt.Sprintf(
		"[%s] [%s] %s\n%s\n",
		notification.Spec.Priority,
		notification.Spec.Type,
		notification.Spec.Subject,
		notification.Spec.Body,
	)

	// Print to stdout
	fmt.Print(formattedMessage)

	return nil
}

