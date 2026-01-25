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

package delivery

import (
	"context"
	"fmt"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ConsoleDeliveryService delivers notifications to console (stdout)
// Uses controller-runtime logging via context (ADR: LOGGING_STANDARD.md)
type ConsoleDeliveryService struct{}

// NewConsoleDeliveryService creates a new console delivery service
func NewConsoleDeliveryService() *ConsoleDeliveryService {
	return &ConsoleDeliveryService{}
}

// Deliver delivers a notification to console (stdout)
// BR-NOT-053: At-least-once delivery (console always succeeds)
// BR-NOT-011: Delivery Error Handling - Input validation
func (s *ConsoleDeliveryService) Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	// TDD GREEN: Validate input (nil-check)
	if notification == nil {
		return fmt.Errorf("notification cannot be nil")
	}

	// Format for console output
	formattedMessage := fmt.Sprintf(
		"[%s] [%s] %s\n%s\n",
		notification.Spec.Priority,
		notification.Spec.Type,
		notification.Spec.Subject,
		notification.Spec.Body,
	)

	// Log to controller-runtime logger (ADR: LOGGING_STANDARD.md)
	logger := log.FromContext(ctx)
	logger.Info(formattedMessage)

	return nil
}
