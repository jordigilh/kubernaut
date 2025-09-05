package errors

import (
	"fmt"
	"strings"
	"testing"
)

func TestOperationError(t *testing.T) {
	tests := []struct {
		name     string
		err      *OperationError
		expected string
	}{
		{
			name: "full error",
			err: &OperationError{
				Operation: "connect to database",
				Component: "postgres",
				Resource:  "user_table",
				Cause:     fmt.Errorf("connection timeout"),
			},
			expected: "failed to connect to database, component: postgres, resource: user_table, cause: connection timeout",
		},
		{
			name: "minimal error",
			err: &OperationError{
				Operation: "parse config",
				Cause:     fmt.Errorf("invalid yaml"),
			},
			expected: "failed to parse config, cause: invalid yaml",
		},
		{
			name: "no cause",
			err: &OperationError{
				Operation: "validate input",
				Component: "validator",
			},
			expected: "failed to validate input, component: validator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("OperationError.Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestOperationError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("underlying error")
	err := &OperationError{
		Operation: "test",
		Cause:     cause,
	}

	if unwrapped := err.Unwrap(); unwrapped != cause {
		t.Errorf("OperationError.Unwrap() = %v, want %v", unwrapped, cause)
	}

	errNoCause := &OperationError{Operation: "test"}
	if unwrapped := errNoCause.Unwrap(); unwrapped != nil {
		t.Errorf("OperationError.Unwrap() with no cause = %v, want nil", unwrapped)
	}
}

func TestFailedTo(t *testing.T) {
	tests := []struct {
		name     string
		action   string
		cause    error
		expected string
	}{
		{
			name:     "with cause",
			action:   "connect to database",
			cause:    fmt.Errorf("connection refused"),
			expected: "failed to connect to database: connection refused",
		},
		{
			name:     "without cause",
			action:   "start server",
			cause:    nil,
			expected: "failed to start server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FailedTo(tt.action, tt.cause)
			if err.Error() != tt.expected {
				t.Errorf("FailedTo() = %q, want %q", err.Error(), tt.expected)
			}
		})
	}
}

func TestFailedToWithDetails(t *testing.T) {
	cause := fmt.Errorf("timeout")
	err := FailedToWithDetails("query users", "database", "users_table", cause)

	opErr, ok := err.(*OperationError)
	if !ok {
		t.Fatalf("FailedToWithDetails() should return *OperationError, got %T", err)
	}

	if opErr.Operation != "query users" {
		t.Errorf("Operation = %q, want %q", opErr.Operation, "query users")
	}
	if opErr.Component != "database" {
		t.Errorf("Component = %q, want %q", opErr.Component, "database")
	}
	if opErr.Resource != "users_table" {
		t.Errorf("Resource = %q, want %q", opErr.Resource, "users_table")
	}
	if opErr.Cause != cause {
		t.Errorf("Cause = %v, want %v", opErr.Cause, cause)
	}
}

func TestWrapf(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		format   string
		args     []interface{}
		expected string
	}{
		{
			name:     "wrap with message",
			err:      fmt.Errorf("original error"),
			format:   "additional context: %s",
			args:     []interface{}{"test"},
			expected: "additional context: test: original error",
		},
		{
			name:     "nil error",
			err:      nil,
			format:   "should not wrap",
			args:     nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Wrapf(tt.err, tt.format, tt.args...)
			if tt.err == nil {
				if result != nil {
					t.Errorf("Wrapf(nil, ...) = %v, want nil", result)
				}
			} else {
				if result.Error() != tt.expected {
					t.Errorf("Wrapf() = %q, want %q", result.Error(), tt.expected)
				}
			}
		})
	}
}

func TestDatabaseError(t *testing.T) {
	cause := fmt.Errorf("connection lost")
	err := DatabaseError("insert record", cause)

	if !strings.Contains(err.Error(), "failed to insert record") {
		t.Errorf("DatabaseError should contain operation, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "database") {
		t.Errorf("DatabaseError should contain component, got %q", err.Error())
	}
}

func TestNetworkError(t *testing.T) {
	cause := fmt.Errorf("timeout")
	err := NetworkError("connect", "https://api.example.com", cause)

	if !strings.Contains(err.Error(), "failed to connect") {
		t.Errorf("NetworkError should contain operation, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "network") {
		t.Errorf("NetworkError should contain component, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "https://api.example.com") {
		t.Errorf("NetworkError should contain endpoint, got %q", err.Error())
	}
}

func TestValidationError(t *testing.T) {
	err := ValidationError("email", "invalid format")
	expected := "validation failed for field email: invalid format"

	if err.Error() != expected {
		t.Errorf("ValidationError() = %q, want %q", err.Error(), expected)
	}
}

func TestConfigurationError(t *testing.T) {
	err := ConfigurationError("database.host", "value is required")
	expected := "configuration error for setting database.host: value is required"

	if err.Error() != expected {
		t.Errorf("ConfigurationError() = %q, want %q", err.Error(), expected)
	}
}

func TestTimeoutError(t *testing.T) {
	err := TimeoutError("waiting for response", "30s")
	expected := "timeout while waiting for response after 30s"

	if err.Error() != expected {
		t.Errorf("TimeoutError() = %q, want %q", err.Error(), expected)
	}
}

func TestAuthenticationError(t *testing.T) {
	err := AuthenticationError("invalid credentials")
	expected := "authentication failed: invalid credentials"

	if err.Error() != expected {
		t.Errorf("AuthenticationError() = %q, want %q", err.Error(), expected)
	}
}

func TestAuthorizationError(t *testing.T) {
	err := AuthorizationError("delete", "user records")
	expected := "authorization failed: insufficient permissions to delete user records"

	if err.Error() != expected {
		t.Errorf("AuthorizationError() = %q, want %q", err.Error(), expected)
	}
}

func TestParseError(t *testing.T) {
	cause := fmt.Errorf("unexpected character")
	err := ParseError("config file", "YAML", cause)

	if !strings.Contains(err.Error(), "parse config file as YAML") {
		t.Errorf("ParseError should contain parse operation, got %q", err.Error())
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "timeout error",
			err:      fmt.Errorf("request timeout"),
			expected: true,
		},
		{
			name:     "connection refused",
			err:      fmt.Errorf("connection refused by server"),
			expected: true,
		},
		{
			name:     "service unavailable",
			err:      fmt.Errorf("service unavailable"),
			expected: true,
		},
		{
			name:     "permanent error",
			err:      fmt.Errorf("invalid syntax"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.expected {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestChain(t *testing.T) {
	tests := []struct {
		name     string
		errors   []error
		expected string
		isNil    bool
	}{
		{
			name:   "no errors",
			errors: []error{nil, nil},
			isNil:  true,
		},
		{
			name:     "single error",
			errors:   []error{fmt.Errorf("single error"), nil},
			expected: "single error",
		},
		{
			name:     "multiple errors",
			errors:   []error{fmt.Errorf("error 1"), fmt.Errorf("error 2"), nil, fmt.Errorf("error 3")},
			expected: "multiple errors: error 1; error 2; error 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Chain(tt.errors...)
			if tt.isNil {
				if result != nil {
					t.Errorf("Chain() = %v, want nil", result)
				}
			} else {
				if result.Error() != tt.expected {
					t.Errorf("Chain() = %q, want %q", result.Error(), tt.expected)
				}
			}
		})
	}
}

