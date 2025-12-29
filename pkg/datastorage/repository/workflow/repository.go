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

package workflow

import (
	"github.com/go-logr/logr"
	"github.com/jmoiron/sqlx"
)

// ========================================
// WORKFLOW REPOSITORY
// ========================================
// Authority: DD-STORAGE-008 v2.0 (Workflow Catalog Schema)
// Business Requirement: BR-STORAGE-012 (Workflow Semantic Search)
// Design Decision: DD-WORKFLOW-002 (MCP Workflow Catalog Architecture)
//
// V1.1 REFACTORING: Split from monolithic workflow_repository.go (1,173 lines)
// into focused modules for better maintainability
// ========================================

// Repository handles workflow catalog operations
// V1.0: Label-only search architecture (no embeddings)
type Repository struct {
	db     *sqlx.DB
	logger logr.Logger
}

// NewRepository creates a new workflow repository
// V1.0: Label-only search (embedding client removed)
func NewRepository(db *sqlx.DB, logger logr.Logger) *Repository {
	return &Repository{
		db:     db,
		logger: logger,
	}
}
