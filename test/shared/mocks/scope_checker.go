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

// IsManaged always returns (true, nil).
func (m *AlwaysManagedScopeChecker) IsManaged(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}

// NeverManagedScopeChecker is a mock that always returns managed=false.
// Use in tests that specifically validate unmanaged resource blocking.
type NeverManagedScopeChecker struct{}

// IsManaged always returns (false, nil).
func (m *NeverManagedScopeChecker) IsManaged(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}

// ErrorScopeChecker is a mock that always returns an error.
// Use in tests that validate graceful degradation on scope infrastructure failures.
type ErrorScopeChecker struct {
	Err error
}

// IsManaged always returns (false, error).
func (m *ErrorScopeChecker) IsManaged(_ context.Context, _, _, _ string) (bool, error) {
	if m.Err != nil {
		return false, m.Err
	}
	return false, fmt.Errorf("scope infrastructure error")
}
