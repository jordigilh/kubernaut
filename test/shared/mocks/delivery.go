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

package mocks

import (
	"context"
	"sync"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// MockDeliveryService is a test double for notification delivery services
// Used in integration tests to simulate delivery behavior without external dependencies
type MockDeliveryService struct {
	// DeliverFunc is the function to execute when Deliver is called
	// If nil, returns nil (success)
	DeliverFunc func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error

	// CallCount tracks how many times Deliver was called
	CallCount int

	// Calls tracks all delivery attempts for assertion
	Calls []DeliveryCall

	// mu protects CallCount and Calls for concurrent access
	mu sync.Mutex
}

// DeliveryCall records details of a single delivery attempt
type DeliveryCall struct {
	Notification *notificationv1alpha1.NotificationRequest
	Error        error
}

// Deliver implements the delivery.Service interface
func (m *MockDeliveryService) Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallCount++

	var err error
	if m.DeliverFunc != nil {
		err = m.DeliverFunc(ctx, notification)
	}

	m.Calls = append(m.Calls, DeliveryCall{
		Notification: notification,
		Error:        err,
	})

	return err
}

// Reset clears call history (useful for test cleanup)
func (m *MockDeliveryService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallCount = 0
	m.Calls = nil
}

// GetCallCount returns the number of times Deliver was called (thread-safe)
func (m *MockDeliveryService) GetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.CallCount
}

// GetCalls returns a copy of all delivery calls (thread-safe)
func (m *MockDeliveryService) GetCalls() []DeliveryCall {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return a copy to prevent race conditions
	calls := make([]DeliveryCall, len(m.Calls))
	copy(calls, m.Calls)
	return calls
}
