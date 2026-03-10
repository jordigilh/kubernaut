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

package actiontype

import (
	"github.com/go-logr/logr"
	"github.com/jmoiron/sqlx"
)

// Repository handles action type taxonomy CRUD operations.
// BR-WORKFLOW-007: ActionType CRD lifecycle management.
// ADR-059: ActionType CRD lifecycle via Admission Webhook.
type Repository struct {
	db     *sqlx.DB
	logger logr.Logger
}

// NewRepository creates a new action type repository.
func NewRepository(db *sqlx.DB, logger logr.Logger) *Repository {
	return &Repository{
		db:     db,
		logger: logger,
	}
}
