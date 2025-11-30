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

package validation

import "github.com/go-logr/logr"

// ValidationRules defines configurable validation rules for audit fields
// BR-STORAGE-010: Configurable validation rules
type ValidationRules struct {
	MaxNameLength       int
	MaxNamespaceLength  int
	MaxActionTypeLength int
	ValidPhases         []string
	ValidStatuses       []string
}

// DefaultRules returns default validation rules
func DefaultRules() *ValidationRules {
	return &ValidationRules{
		MaxNameLength:       255,
		MaxNamespaceLength:  255,
		MaxActionTypeLength: 100,
		ValidPhases:         []string{"pending", "processing", "completed", "failed"},
		ValidStatuses:       []string{"success", "failure", "pending", "running"},
	}
}

// NewValidatorWithRules creates a validator with custom rules
func NewValidatorWithRules(logger logr.Logger, rules *ValidationRules) *Validator {
	// For now, we use default rules internally
	// This allows future extension without breaking existing code
	_ = rules // Will be used in future enhancement
	return &Validator{
		logger: logger,
	}
}
