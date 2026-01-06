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

// ========================================
// INTERFACE COMPLIANCE VERIFICATION
// ========================================
//
// The ImmuClient interface is large, but we only need these methods:
// - VerifiedSet(ctx, key, value) - For audit event writes
// - CurrentState(ctx) - For health checks
// - HealthCheck(ctx) - For server connectivity checks (Phase 5.2)
// - CloseSession(ctx) - For graceful shutdown (Phase 5.2)
// - Login(ctx, user, pass) - For authentication (Phase 5.2)
//
// Future phases will add:
// - VerifiedGet(ctx, key) - For audit event reads (Phase 5.3)
// - Scan(ctx, prefix) - For correlation_id queries (Phase 5.3)
//
// ========================================

