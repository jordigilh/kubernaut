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

// CircuitBreakerService wraps any delivery.Service with circuit breaker protection.
// BR-NOT-055: Graceful Degradation - prevents cascading failures when a channel's
// upstream API is unhealthy. The breakerName parameter isolates breaker state per
// channel type (e.g. "slack", "pagerduty", "teams").
type CircuitBreakerService struct {
	inner       Service
	cbManager   *circuitbreaker.Manager
	breakerName string
}

var _ Service = (*CircuitBreakerService)(nil)

// NewCircuitBreakerService creates a delivery service wrapped with circuit breaker protection.
// breakerName identifies the breaker in the shared Manager (e.g. "slack", "pagerduty", "teams").
func NewCircuitBreakerService(inner Service, cbManager *circuitbreaker.Manager, breakerName string) *CircuitBreakerService {
	return &CircuitBreakerService{
		inner:       inner,
		cbManager:   cbManager,
		breakerName: breakerName,
	}
}

// Deliver sends the notification through the circuit breaker.
// When the circuit is open, returns gobreaker.ErrOpenState.
func (s *CircuitBreakerService) Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	_, err := s.cbManager.Execute(s.breakerName, func() (interface{}, error) {
		return nil, s.inner.Deliver(ctx, notification)
	})
	if err == gobreaker.ErrOpenState {
		return err
	}
	return err
}
