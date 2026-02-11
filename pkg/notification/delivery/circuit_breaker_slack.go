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

	"github.com/sony/gobreaker"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/circuitbreaker"
)

// CircuitBreakerSlackService wraps SlackDeliveryService with circuit breaker protection.
// BR-NOT-055: Graceful Degradation - prevents cascading failures when Slack API is unhealthy.
// Failures are tracked via Execute() so the circuit trips after consecutive failures.
type CircuitBreakerSlackService struct {
	slack    *SlackDeliveryService
	cbManager *circuitbreaker.Manager
}

// NewCircuitBreakerSlackService creates a Slack delivery service with circuit breaker protection.
func NewCircuitBreakerSlackService(slack *SlackDeliveryService, cbManager *circuitbreaker.Manager) *CircuitBreakerSlackService {
	return &CircuitBreakerSlackService{
		slack:     slack,
		cbManager: cbManager,
	}
}

// Deliver sends the notification to Slack via the circuit breaker.
// When the circuit is open, returns gobreaker.ErrOpenState (handled by checkBeforeDelivery + CircuitBreakerOpen event).
func (s *CircuitBreakerSlackService) Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	_, err := s.cbManager.Execute("slack", func() (interface{}, error) {
		return nil, s.slack.Deliver(ctx, notification)
	})
	if err == gobreaker.ErrOpenState {
		return err
	}
	return err
}
