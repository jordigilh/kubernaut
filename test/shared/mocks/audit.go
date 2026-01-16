package mocks

import (
	"context"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// MockAuditStore implements audit.AuditStore interface for testing
// This is a shared mock for all services to use in unit tests.
//
// Usage:
//
//	mockStore := mocks.NewMockAuditStore()
//	mockStore.StoreAuditFunc = func(ctx context.Context, event *ogenclient.AuditEventRequest) error {
//	    return errors.New("audit unavailable") // Simulate failure
//	}
//
// Features:
// - Customizable behavior via function fields
// - Call tracking (storeCalls, flushCalls, closeCalls)
// - Default success behavior
// - Thread-safe for concurrent tests
type MockAuditStore struct {
	// Customizable behavior
	StoreAuditFunc func(ctx context.Context, event *ogenclient.AuditEventRequest) error
	FlushFunc      func(ctx context.Context) error
	CloseFunc      func() error

	// Call tracking
	storeCalls int
	flushCalls int
	closeCalls int

	// Event storage (if needed for assertions)
	StoredEvents []*ogenclient.AuditEventRequest
}

// NewMockAuditStore creates a new mock audit store with default success behavior.
// Customize behavior by overriding the function fields (StoreAuditFunc, FlushFunc, CloseFunc).
func NewMockAuditStore() *MockAuditStore {
	return &MockAuditStore{
		StoreAuditFunc: func(ctx context.Context, event *ogenclient.AuditEventRequest) error {
			return nil // Default: success
		},
		FlushFunc: func(ctx context.Context) error {
			return nil // Default: success
		},
		CloseFunc: func() error {
			return nil // Default: success
		},
		StoredEvents: make([]*ogenclient.AuditEventRequest, 0),
	}
}

// StoreAudit implements audit.AuditStore interface
func (m *MockAuditStore) StoreAudit(ctx context.Context, event *ogenclient.AuditEventRequest) error {
	m.storeCalls++
	if err := m.StoreAuditFunc(ctx, event); err != nil {
		return err
	}
	// Store event for later assertions
	m.StoredEvents = append(m.StoredEvents, event)
	return nil
}

// Flush implements audit.AuditStore interface
func (m *MockAuditStore) Flush(ctx context.Context) error {
	m.flushCalls++
	return m.FlushFunc(ctx)
}

// Close implements audit.AuditStore interface
func (m *MockAuditStore) Close() error {
	m.closeCalls++
	return m.CloseFunc()
}

// GetStoreCalls returns the number of times StoreAudit was called
func (m *MockAuditStore) GetStoreCalls() int {
	return m.storeCalls
}

// GetFlushCalls returns the number of times Flush was called
func (m *MockAuditStore) GetFlushCalls() int {
	return m.flushCalls
}

// GetCloseCalls returns the number of times Close was called
func (m *MockAuditStore) GetCloseCalls() int {
	return m.closeCalls
}

// Reset resets all call counters and stored events (useful for BeforeEach)
func (m *MockAuditStore) Reset() {
	m.storeCalls = 0
	m.flushCalls = 0
	m.closeCalls = 0
	m.StoredEvents = make([]*ogenclient.AuditEventRequest, 0)
}

// GetEventsByType filters stored events by event type
func (m *MockAuditStore) GetEventsByType(eventType string) []*ogenclient.AuditEventRequest {
	var filtered []*ogenclient.AuditEventRequest
	for _, event := range m.StoredEvents {
		if event.EventType == eventType {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// GetLastEvent returns the last stored event (or nil if none)
func (m *MockAuditStore) GetLastEvent() *ogenclient.AuditEventRequest {
	if len(m.StoredEvents) == 0 {
		return nil
	}
	return m.StoredEvents[len(m.StoredEvents)-1]
}
