package gateway_test

import (
	"context"
	"sync"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// MockAlertProcessorClient implements AlertProcessorClient interface for testing
// Following established mock patterns from pkg/testutil/mocks/
type MockAlertProcessorClient struct {
	forwardAlertFunc      func(ctx context.Context, alert *types.Alert) error
	healthCheckFunc       func(ctx context.Context) error
	forwardAlertCallCount int
	healthCheckCallCount  int
	mu                    sync.RWMutex
}

// NewMockAlertProcessorClient creates a new mock Alert Processor client
func NewMockAlertProcessorClient() *MockAlertProcessorClient {
	return &MockAlertProcessorClient{
		forwardAlertFunc: func(ctx context.Context, alert *types.Alert) error {
			return nil
		},
		healthCheckFunc: func(ctx context.Context) error {
			return nil
		},
	}
}

// ForwardAlert mocks forwarding alert to Alert Processor service
func (m *MockAlertProcessorClient) ForwardAlert(ctx context.Context, alert *types.Alert) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forwardAlertCallCount++
	return m.forwardAlertFunc(ctx, alert)
}

// HealthCheck mocks health check to Alert Processor service
func (m *MockAlertProcessorClient) HealthCheck(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthCheckCallCount++
	return m.healthCheckFunc(ctx)
}

// ExpectForwardAlert sets up expectation for ForwardAlert call
func (m *MockAlertProcessorClient) ExpectForwardAlert() *MockAlertProcessorClient {
	m.forwardAlertFunc = func(ctx context.Context, alert *types.Alert) error {
		return nil
	}
	return m
}

// Return sets the return value for the expected call
func (m *MockAlertProcessorClient) Return(err error) *MockAlertProcessorClient {
	m.forwardAlertFunc = func(ctx context.Context, alert *types.Alert) error {
		return err
	}
	return m
}

// ForwardAlertCallCount returns the number of times ForwardAlert was called
func (m *MockAlertProcessorClient) ForwardAlertCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.forwardAlertCallCount
}

// HealthCheckCallCount returns the number of times HealthCheck was called
func (m *MockAlertProcessorClient) HealthCheckCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.healthCheckCallCount
}

// Reset resets all call counts and expectations
func (m *MockAlertProcessorClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forwardAlertCallCount = 0
	m.healthCheckCallCount = 0
	m.forwardAlertFunc = func(ctx context.Context, alert *types.Alert) error {
		return nil
	}
	m.healthCheckFunc = func(ctx context.Context) error {
		return nil
	}
}

