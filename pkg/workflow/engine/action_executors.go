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
package engine

import (
	"context"
)

// ActionHandler defines the function signature for action handlers
type ActionHandler func(ctx context.Context, action *StepAction, stepContext *StepContext) (*StepResult, error)

// Note: The specific ActionExecutor implementations (KubernetesActionExecutor,
// MonitoringActionExecutor, CustomActionExecutor) have been moved to separate files:
// - kubernetes_action_executor.go
// - monitoring_action_executor.go
// - custom_action_executor.go
// This prevents duplicate declarations and follows better code organization.
