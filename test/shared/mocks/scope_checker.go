package mocks

import (
	"context"
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// Compile-time interface compliance checks.
var _ scope.ScopeChecker = (*AlwaysManagedScopeChecker)(nil)
var _ scope.ScopeChecker = (*NeverManagedScopeChecker)(nil)
var _ scope.ScopeChecker = (*ErrorScopeChecker)(nil)

// AlwaysManagedScopeChecker is a mock that always returns managed=true.
// Use in tests where scope validation is not the focus and all resources
// should pass through.
type AlwaysManagedScopeChecker struct{}

// Check always returns (true, nil).
func (m *AlwaysManagedScopeChecker) Check(_ context.Context, _ scope.ResourceIdentity) (scope.ScopeResult, error) {
	return scope.ScopeResult{State: scope.ScopeVerifiedManaged, Provenance: scope.ScopeSourceResourceLabel}, nil
}

// NeverManagedScopeChecker is a mock that always returns managed=false.
// Use in tests that specifically validate unmanaged resource blocking.
type NeverManagedScopeChecker struct{}

// Check always returns (false, nil).
func (m *NeverManagedScopeChecker) Check(_ context.Context, _ scope.ResourceIdentity) (scope.ScopeResult, error) {
	return scope.ScopeResult{State: scope.ScopeVerifiedUnmanaged, Provenance: scope.ScopeSourceResourceLabel}, nil
}

// ErrorScopeChecker is a mock that always returns an error.
// Use in tests that validate graceful degradation on scope infrastructure failures.
type ErrorScopeChecker struct {
	Err error
}

// Check always returns (false, error).
func (m *ErrorScopeChecker) Check(_ context.Context, _ scope.ResourceIdentity) (scope.ScopeResult, error) {
	if m.Err != nil {
		return scope.ScopeResult{State: scope.ScopeUnavailable, Provenance: scope.ScopeSourceUnavailable}, m.Err
	}
	return scope.ScopeResult{State: scope.ScopeUnavailable, Provenance: scope.ScopeSourceUnavailable}, fmt.Errorf("scope infrastructure error")
}
