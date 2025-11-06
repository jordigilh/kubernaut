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
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// Vector is a custom type for pgvector embeddings that implements sql.Scanner
// BR-STORAGE-008: Vector type for pgvector compatibility
type Vector []float32

// Scan implements sql.Scanner for Vector type
// Converts PostgreSQL vector format "[x,y,z,...]" to []float32
func (v *Vector) Scan(value interface{}) error {
	if value == nil {
		*v = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to scan Vector: expected []byte, got %T", value)
	}

	// Parse "[x,y,z,...]" format
	str := string(bytes)
	str = strings.TrimPrefix(str, "[")
	str = strings.TrimSuffix(str, "]")

	if str == "" {
		*v = []float32{}
		return nil
	}

	parts := strings.Split(str, ",")
	result := make([]float32, len(parts))

	for i, part := range parts {
		val, err := strconv.ParseFloat(strings.TrimSpace(part), 32)
		if err != nil {
			return fmt.Errorf("failed to parse vector element %d: %w", i, err)
		}
		result[i] = float32(val)
	}

	*v = result
	return nil
}

// Value implements driver.Valuer for Vector type
// Converts []float32 to PostgreSQL vector format "[x,y,z,...]"
func (v Vector) Value() (driver.Value, error) {
	if v == nil {
		return nil, nil
	}

	if len(v) == 0 {
		return "[]", nil
	}

	parts := make([]string, len(v))
	for i, val := range v {
		parts[i] = strconv.FormatFloat(float64(val), 'f', -1, 32)
	}

	return "[" + strings.Join(parts, ",") + "]", nil
}

// RemediationAuditResult is used for scanning from database with Vector support
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
	Embedding            Vector     `db:"embedding"` // Use Vector for scanning
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
		Embedding:            models.Vector(r.Embedding), // Convert query.Vector to models.Vector
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}

// SemanticResultRow is used for scanning semantic search results from database
type SemanticResultRow struct {
	RemediationAuditResult
	Similarity float32 `db:"similarity"`
}

// ToSemanticResult converts SemanticResultRow to SemanticResult
func (r *SemanticResultRow) ToSemanticResult() *SemanticResult {
	return &SemanticResult{
		RemediationAudit: r.ToRemediationAudit(),
		Similarity:       r.Similarity,
	}
}

// SemanticResult represents a semantic search result with similarity score
// BR-STORAGE-012: Semantic search result type
type SemanticResult struct {
	*models.RemediationAudit
	Similarity float32 `json:"similarity"` // Cosine similarity score (0-1)
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
