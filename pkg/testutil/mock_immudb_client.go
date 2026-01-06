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

package testutil

import (
	"context"
	"fmt"

	immuschema "github.com/codenotary/immudb/pkg/api/schema"
)

// ========================================
// MOCK IMMUDB CLIENT - Unit Testing Support
// 📋 SOC2 Gap #9: Tamper-Evident Audit Trail - Phase 5.1
// Purpose: Lightweight mock for unit tests (no external dependencies)
// ========================================
//
// This mock implements the minimum Immudb client interface needed for unit tests.
// Integration tests will use real Immudb containers.
//
// TESTING STRATEGY (Defense-in-Depth):
// - **Unit Tests**: Use this mock (fast, isolated, no infrastructure)
// - **Integration Tests**: Real Immudb containers (Phases 5.2-5.4)
// - **E2E Tests**: Full DataStorage service with Immudb
//
// ========================================

// MockImmudbClient is a test double for immudb.ImmuClient
// Only implements methods used by ImmudbAuditEventsRepository
type MockImmudbClient struct {
	// VerifiedSet tracking
	VerifiedSetCalls  []VerifiedSetCall
	VerifiedSetError  error
	VerifiedSetResult *immuschema.TxHeader

	// CurrentState tracking (for HealthCheck)
	CurrentStateCalls int
	CurrentStateError error
	CurrentStateResult *immuschema.ImmutableState
}

// VerifiedSetCall captures the arguments of a VerifiedSet call
type VerifiedSetCall struct {
	Ctx   context.Context
	Key   []byte
	Value []byte
}

// NewMockImmudbClient creates a new mock Immudb client
func NewMockImmudbClient() *MockImmudbClient {
	return &MockImmudbClient{
		VerifiedSetCalls: make([]VerifiedSetCall, 0),
		// Default: Return success with transaction ID 1
		VerifiedSetResult: &immuschema.TxHeader{
			Id: 1, // Monotonic transaction ID
		},
		CurrentStateResult: &immuschema.ImmutableState{
			TxId: 1,
		},
	}
}

// VerifiedSet mocks the Immudb VerifiedSet operation
// Records call arguments and returns configured result/error
func (m *MockImmudbClient) VerifiedSet(ctx context.Context, key []byte, value []byte) (*immuschema.TxHeader, error) {
	// Record call for verification
	m.VerifiedSetCalls = append(m.VerifiedSetCalls, VerifiedSetCall{
		Ctx:   ctx,
		Key:   key,
		Value: value,
	})

	// Return configured error if set
	if m.VerifiedSetError != nil {
		return nil, m.VerifiedSetError
	}

	// Auto-increment transaction ID for multiple calls
	if len(m.VerifiedSetCalls) > 1 {
		m.VerifiedSetResult.Id = uint64(len(m.VerifiedSetCalls))
	}

	return m.VerifiedSetResult, nil
}

// CurrentState mocks the Immudb CurrentState operation (used for health checks)
func (m *MockImmudbClient) CurrentState(ctx context.Context) (*immuschema.ImmutableState, error) {
	m.CurrentStateCalls++

	if m.CurrentStateError != nil {
		return nil, m.CurrentStateError
	}

	return m.CurrentStateResult, nil
}

// HealthCheck mocks the Immudb HealthCheck operation (Phase 5.2)
func (m *MockImmudbClient) HealthCheck(ctx context.Context) error {
	// Simple mock: always return nil (success)
	return nil
}

// CloseSession mocks the Immudb CloseSession operation (Phase 5.2)
func (m *MockImmudbClient) CloseSession(ctx context.Context) error {
	// Simple mock: always return nil (success)
	return nil
}

// Login mocks the Immudb Login operation (Phase 5.2)
func (m *MockImmudbClient) Login(ctx context.Context, user []byte, password []byte) (*immuschema.LoginResponse, error) {
	// Simple mock: return empty LoginResponse (success)
	return &immuschema.LoginResponse{}, nil
}

// VerifiedGet mocks the Immudb VerifiedGet operation (Phase 5.3)
func (m *MockImmudbClient) VerifiedGet(ctx context.Context, key []byte, opts ...interface{}) (*immuschema.Entry, error) {
	// Simple mock: return nil (not found)
	return nil, fmt.Errorf("key not found")
}

// Scan mocks the Immudb Scan operation (Phase 5.3)
func (m *MockImmudbClient) Scan(ctx context.Context, req *immuschema.ScanRequest) (*immuschema.Entries, error) {
	// Simple mock: return empty entries
	return &immuschema.Entries{
		Entries: []*immuschema.Entry{},
	}, nil
}

// SetAll mocks the Immudb SetAll operation (Phase 5.3)
func (m *MockImmudbClient) SetAll(ctx context.Context, req *immuschema.SetRequest) (*immuschema.TxHeader, error) {
	// Simple mock: return success with transaction ID
	return &immuschema.TxHeader{
		Id: 1,
	}, nil
}

// ========================================
// NO-OP STUBS FOR client.ImmuClient INTERFACE COMPLIANCE
// ========================================
//
// The client.ImmuClient interface has 50+ methods.
// We only use a small subset for audit storage.
// These no-op stubs satisfy the interface but are never called in unit tests.
//
// ========================================

// ChangePassword is a no-op stub (not used in audit storage)
func (m *MockImmudbClient) ChangePassword(ctx context.Context, user []byte, oldPass []byte, newPass []byte) error {
	panic("ChangePassword should not be called in audit storage unit tests")
}

// CreateUser is a no-op stub (not used in audit storage)
func (m *MockImmudbClient) CreateUser(ctx context.Context, user []byte, pass []byte, permission uint32, databasename string) error {
	panic("CreateUser should not be called in audit storage unit tests")
}

// IsConnected is a no-op stub (not used in audit storage)
func (m *MockImmudbClient) IsConnected() bool {
	return true
}

// Disconnect is a no-op stub (not used in audit storage)
func (m *MockImmudbClient) Disconnect() error {
	return nil
}

// GetSessionID is a no-op stub (not used in audit storage)
func (m *MockImmudbClient) GetSessionID() string {
	return "mock-session-id"
}

// ========================================
// INTERFACE COMPLIANCE VERIFICATION
// ========================================
//
// The ImmuClient interface is large (50+ methods), but we only need these for audit storage:
// - VerifiedSet(ctx, key, value) - For audit event writes (Phase 5.1)
// - CurrentState(ctx) - For health checks (Phase 5.1)
// - HealthCheck(ctx) - For server connectivity checks (Phase 5.2)
// - CloseSession(ctx) - For graceful shutdown (Phase 5.2)
// - Login(ctx, user, pass) - For authentication (Phase 5.2)
// - VerifiedGet(ctx, key) - For audit event reads (Phase 5.3)
// - Scan(ctx, prefix) - For correlation_id queries (Phase 5.3)
// - SetAll(ctx, req) - For batch writes (Phase 5.3)
//
// All other methods are no-op stubs that panic if called.
//
// ========================================

