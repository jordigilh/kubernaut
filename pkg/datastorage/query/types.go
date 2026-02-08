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

package query

import (
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// RemediationAuditResult is used for scanning from database
// This is an internal type that gets converted to models.RemediationAudit
type RemediationAuditResult struct {
	ID                   int64      `db:"id"`
	Name                 string     `db:"name"`
	Namespace            string     `db:"namespace"`
	Phase                string     `db:"phase"`
	ActionType           string     `db:"action_type"`
	Status               string     `db:"status"`
	StartTime            time.Time  `db:"start_time"`
	EndTime              *time.Time `db:"end_time"`
	Duration             *int64     `db:"duration"`
	RemediationRequestID string     `db:"remediation_request_id"`
	SignalFingerprint    string     `db:"signal_fingerprint"`
	Severity             string     `db:"severity"`
	Environment          string     `db:"environment"`
	ClusterName          string     `db:"cluster_name"`
	TargetResource       string     `db:"target_resource"`
	ErrorMessage         *string    `db:"error_message"`
	Metadata             string     `db:"metadata"`
	CreatedAt            time.Time  `db:"created_at"`
	UpdatedAt            time.Time  `db:"updated_at"`
}

// ToRemediationAudit converts RemediationAuditResult to models.RemediationAudit
func (r *RemediationAuditResult) ToRemediationAudit() *models.RemediationAudit {
	return &models.RemediationAudit{
		ID:                   r.ID,
		Name:                 r.Name,
		Namespace:            r.Namespace,
		Phase:                r.Phase,
		ActionType:           r.ActionType,
		Status:               r.Status,
		StartTime:            r.StartTime,
		EndTime:              r.EndTime,
		Duration:             r.Duration,
		RemediationRequestID: r.RemediationRequestID,
		SignalFingerprint:    r.SignalFingerprint,
		Severity:             r.Severity,
		Environment:          r.Environment,
		ClusterName:          r.ClusterName,
		TargetResource:       r.TargetResource,
		ErrorMessage:         r.ErrorMessage,
		Metadata:             r.Metadata,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

// PaginationResult contains paginated results with metadata
// BR-STORAGE-006: Pagination support
type PaginationResult struct {
	Data       interface{} `json:"data"`
	TotalCount int64       `json:"total_count"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}
