package mocks

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/middleware"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// MockAlertProcessorClient implements AlertProcessorClient interface for testing
// Following established mock patterns from existing mocks in this package
type MockAlertProcessorClient struct {
	forwardAlertFunc      func(ctx context.Context, alert *types.Alert) error
	healthCheckFunc       func(ctx context.Context) error
	forwardAlertCallCount int
	healthCheckCallCount  int
	simulatedDelay        time.Duration // For timeout testing
	mu                    sync.RWMutex
}

// NewMockAlertProcessorClient creates a new mock Alert Processor client
// Following factory pattern established in this package
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
	delay := m.simulatedDelay
	m.forwardAlertCallCount++
	m.mu.Unlock()

	// Simulate delay if configured
	if delay > 0 {
		select {
		case <-time.After(delay):
			// Delay completed
		case <-ctx.Done():
			// Context cancelled/timed out during delay
			return ctx.Err()
		}
	}

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

// WithDelay configures simulated delay for testing timeout scenarios
func (m *MockAlertProcessorClient) WithDelay(delay time.Duration) *MockAlertProcessorClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.simulatedDelay = delay
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
	m.simulatedDelay = 0
	m.forwardAlertFunc = func(ctx context.Context, alert *types.Alert) error {
		return nil
	}
	m.healthCheckFunc = func(ctx context.Context) error {
		return nil
	}
}

// MockAuthenticator implements the middleware.Authenticator interface for testing OAuth2/JWT
type MockAuthenticator struct {
	ShouldAuthenticate bool
	Username           string
	Namespace          string
	Groups             []string
	AuthError          error
}

// NewMockAuthenticator creates a new mock authenticator for OAuth2/JWT testing
func NewMockAuthenticator() *MockAuthenticator {
	return &MockAuthenticator{
		ShouldAuthenticate: true,
		Username:           "system:serviceaccount:test:test-sa",
		Namespace:          "test",
		Groups:             []string{"system:serviceaccounts", "system:serviceaccounts:test"},
	}
}

// Authenticate implements the middleware.Authenticator interface
func (m *MockAuthenticator) Authenticate(ctx context.Context, r *http.Request) (*middleware.AuthenticationResult, error) {
	if m.AuthError != nil {
		return nil, m.AuthError
	}

	if !m.ShouldAuthenticate {
		return &middleware.AuthenticationResult{
			Authenticated: false,
			Errors:        []string{"mock OAuth2/JWT authentication failed"},
		}, nil
	}

	return &middleware.AuthenticationResult{
		Authenticated: true,
		Username:      m.Username,
		Namespace:     m.Namespace,
		Groups:        m.Groups,
		Metadata: map[string]string{
			"auth_type": "oauth2",
			"mock":      "true",
		},
	}, nil
}

// GetType implements the middleware.Authenticator interface
func (m *MockAuthenticator) GetType() string {
	return "oauth2"
}

// SetShouldAuthenticate configures whether authentication should succeed
func (m *MockAuthenticator) SetShouldAuthenticate(should bool) *MockAuthenticator {
	m.ShouldAuthenticate = should
	return m
}

// SetAuthError configures an authentication error to return
func (m *MockAuthenticator) SetAuthError(err error) *MockAuthenticator {
	m.AuthError = err
	return m
}

// SetUserInfo configures the user information for successful authentication
func (m *MockAuthenticator) SetUserInfo(username, namespace string, groups []string) *MockAuthenticator {
	m.Username = username
	m.Namespace = namespace
	m.Groups = groups
	return m
}
