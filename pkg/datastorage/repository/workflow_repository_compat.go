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

package repository

import (
	"github.com/go-logr/logr"
	"github.com/jmoiron/sqlx"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
)

// ========================================
// COMPATIBILITY LAYER
// ========================================
// V1.1 REFACTORING: This file provides backwards compatibility
// for code that uses WorkflowRepository directly.
//
// The actual implementation has been split into focused modules:
//   - workflow/repository.go - struct and constructor
//   - workflow/crud.go - CRUD operations
//   - workflow/search.go - search operations
//
// This compatibility layer will be removed in V1.2
// ========================================

// WorkflowRepository is a compatibility wrapper around workflow.Repository
// DEPRECATED: Use workflow.NewRepository directly
type WorkflowRepository struct {
	*workflow.Repository
}

// NewWorkflowRepository creates a new workflow repository
// DEPRECATED: Use workflow.NewRepository directly
func NewWorkflowRepository(db *sqlx.DB, logger logr.Logger) *WorkflowRepository {
	return &WorkflowRepository{
		Repository: workflow.NewRepository(db, logger),
	}
}
