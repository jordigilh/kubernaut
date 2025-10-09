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

package types

import "time"

// BaseEntity provides common fields for all identifiable entities
// This eliminates ~200 lines of duplicate field definitions across the codebase
type BaseEntity struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// BaseVersionedEntity extends BaseEntity with versioning
type BaseVersionedEntity struct {
	BaseEntity
	Version   string `json:"version"`
	CreatedBy string `json:"created_by,omitempty"`
}

// BaseTimestampedResult provides common fields for execution results
type BaseTimestampedResult struct {
	Success   bool          `json:"success"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration"`
	Error     string        `json:"error,omitempty"`
}

// BasePattern provides common fields for all discovered patterns
type BasePattern struct {
	BaseEntity
	Type                 string             `json:"type"`
	Confidence           float64            `json:"confidence"`
	Frequency            int                `json:"frequency"`
	SuccessRate          float64            `json:"success_rate"`
	AverageExecutionTime time.Duration      `json:"average_execution_time"`
	LastSeen             time.Time          `json:"last_seen"`
	Tags                 []string           `json:"tags,omitempty"`
	SourceExecutions     []string           `json:"source_executions,omitempty"`
	Metrics              map[string]float64 `json:"metrics,omitempty"`
}

// BaseContext provides common fields for all context types
type BaseContext struct {
	Labels      map[string]string      `json:"labels,omitempty"`
	Annotations map[string]string      `json:"annotations,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Environment string                 `json:"environment,omitempty"`
	Cluster     string                 `json:"cluster,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
}

// BaseResourceInfo provides common fields for resource identification
type BaseResourceInfo struct {
	Namespace string `json:"namespace,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Name      string `json:"name"`
}
