<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package executor

import (
	"context"
	"fmt"
	"sync"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ActionHandler defines the signature for action execution functions
type ActionHandler func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error

// ActionRegistry manages registered actions and their handlers
type ActionRegistry struct {
	handlers map[string]ActionHandler
	mutex    sync.RWMutex
}

// NewActionRegistry creates a new action registry
func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{
		handlers: make(map[string]ActionHandler),
	}
}

// Register adds a new action handler to the registry
func (r *ActionRegistry) Register(actionName string, handler ActionHandler) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.handlers[actionName]; exists {
		return fmt.Errorf("action '%s' is already registered", actionName)
	}

	r.handlers[actionName] = handler
	return nil
}

// Unregister removes an action handler from the registry
func (r *ActionRegistry) Unregister(actionName string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	delete(r.handlers, actionName)
}

// Execute executes the registered handler for the given action
func (r *ActionRegistry) Execute(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	r.mutex.RLock()
	handler, exists := r.handlers[action.Action]
	r.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("unknown action: %s", action.Action)
	}

	return handler(ctx, action, alert)
}

// GetRegisteredActions returns a list of all registered action names
func (r *ActionRegistry) GetRegisteredActions() []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	actions := make([]string, 0, len(r.handlers))
	for actionName := range r.handlers {
		actions = append(actions, actionName)
	}
	return actions
}

// IsRegistered checks if an action is registered
func (r *ActionRegistry) IsRegistered(actionName string) bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	_, exists := r.handlers[actionName]
	return exists
}

// Count returns the number of registered actions
func (r *ActionRegistry) Count() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return len(r.handlers)
}
