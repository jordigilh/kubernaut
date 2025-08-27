package executor

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestNewActionRegistry(t *testing.T) {
	registry := NewActionRegistry()
	
	assert.NotNil(t, registry)
	assert.Equal(t, 0, registry.Count())
}

func TestActionRegistry_Register(t *testing.T) {
	registry := NewActionRegistry()
	
	// Test successful registration
	handler := func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
		return nil
	}
	
	err := registry.Register("test_action", handler)
	assert.NoError(t, err)
	assert.Equal(t, 1, registry.Count())
	assert.True(t, registry.IsRegistered("test_action"))
	
	// Test duplicate registration
	err = registry.Register("test_action", handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestActionRegistry_Unregister(t *testing.T) {
	registry := NewActionRegistry()
	
	handler := func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
		return nil
	}
	
	registry.Register("test_action", handler)
	assert.Equal(t, 1, registry.Count())
	
	registry.Unregister("test_action")
	assert.Equal(t, 0, registry.Count())
	assert.False(t, registry.IsRegistered("test_action"))
	
	// Test unregistering non-existent action (should not panic)
	registry.Unregister("non_existent")
	assert.Equal(t, 0, registry.Count())
}

func TestActionRegistry_Execute(t *testing.T) {
	registry := NewActionRegistry()
	ctx := context.Background()
	
	executed := false
	handler := func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
		executed = true
		return nil
	}
	
	registry.Register("test_action", handler)
	
	action := &types.ActionRecommendation{
		Action: "test_action",
	}
	alert := types.Alert{
		Name: "test_alert",
	}
	
	err := registry.Execute(ctx, action, alert)
	assert.NoError(t, err)
	assert.True(t, executed)
}

func TestActionRegistry_Execute_UnknownAction(t *testing.T) {
	registry := NewActionRegistry()
	ctx := context.Background()
	
	action := &types.ActionRecommendation{
		Action: "unknown_action",
	}
	alert := types.Alert{
		Name: "test_alert",
	}
	
	err := registry.Execute(ctx, action, alert)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown action")
}

func TestActionRegistry_Execute_HandlerError(t *testing.T) {
	registry := NewActionRegistry()
	ctx := context.Background()
	
	expectedError := errors.New("handler error")
	handler := func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
		return expectedError
	}
	
	registry.Register("error_action", handler)
	
	action := &types.ActionRecommendation{
		Action: "error_action",
	}
	alert := types.Alert{
		Name: "test_alert",
	}
	
	err := registry.Execute(ctx, action, alert)
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func TestActionRegistry_GetRegisteredActions(t *testing.T) {
	registry := NewActionRegistry()
	
	// Test empty registry
	actions := registry.GetRegisteredActions()
	assert.Empty(t, actions)
	
	// Test with registered actions
	handler := func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
		return nil
	}
	
	registry.Register("action1", handler)
	registry.Register("action2", handler)
	registry.Register("action3", handler)
	
	actions = registry.GetRegisteredActions()
	assert.Len(t, actions, 3)
	assert.Contains(t, actions, "action1")
	assert.Contains(t, actions, "action2")
	assert.Contains(t, actions, "action3")
}

func TestActionRegistry_IsRegistered(t *testing.T) {
	registry := NewActionRegistry()
	
	handler := func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
		return nil
	}
	
	assert.False(t, registry.IsRegistered("test_action"))
	
	registry.Register("test_action", handler)
	assert.True(t, registry.IsRegistered("test_action"))
	
	registry.Unregister("test_action")
	assert.False(t, registry.IsRegistered("test_action"))
}

func TestActionRegistry_Count(t *testing.T) {
	registry := NewActionRegistry()
	
	handler := func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
		return nil
	}
	
	assert.Equal(t, 0, registry.Count())
	
	registry.Register("action1", handler)
	assert.Equal(t, 1, registry.Count())
	
	registry.Register("action2", handler)
	assert.Equal(t, 2, registry.Count())
	
	registry.Unregister("action1")
	assert.Equal(t, 1, registry.Count())
	
	registry.Unregister("action2")
	assert.Equal(t, 0, registry.Count())
}

func TestActionRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewActionRegistry()
	
	handler := func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
		return nil
	}
	
	// Test concurrent registration and execution
	done := make(chan bool)
	
	// Goroutine 1: Register actions
	go func() {
		for i := 0; i < 10; i++ {
			registry.Register(fmt.Sprintf("action%d", i), handler)
		}
		done <- true
	}()
	
	// Goroutine 2: Check registered actions
	go func() {
		for i := 0; i < 10; i++ {
			registry.GetRegisteredActions()
			registry.Count()
		}
		done <- true
	}()
	
	// Wait for both goroutines to complete
	<-done
	<-done
	
	assert.Equal(t, 10, registry.Count())
}