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

package models

import (
	"time"
)

// RemediationAudit represents a complete remediation workflow audit
// BR-STORAGE-001: Audit trail for remediation workflows
type RemediationAudit struct {
	ID                   int64      `json:"id" db:"id"`
	Name                 string     `json:"name" db:"name"`
	Namespace            string     `json:"namespace" db:"namespace"`
	Phase                string     `json:"phase" db:"phase"` // pending, processing, completed, failed
	ActionType           string     `json:"action_type" db:"action_type"`
	Status               string     `json:"status" db:"status"`
	StartTime            time.Time  `json:"start_time" db:"start_time"`
	EndTime              *time.Time `json:"end_time,omitempty" db:"end_time"`
	Duration             *int64     `json:"duration,omitempty" db:"duration"` // milliseconds
	RemediationRequestID string     `json:"remediation_request_id" db:"remediation_request_id"`
	SignalFingerprint    string     `json:"signal_fingerprint" db:"signal_fingerprint"`
	Severity             string     `json:"severity" db:"severity"`
	Environment          string     `json:"environment" db:"environment"`
	ClusterName          string     `json:"cluster_name" db:"cluster_name"`
	TargetResource       string     `json:"target_resource" db:"target_resource"`
	ErrorMessage         *string    `json:"error_message,omitempty" db:"error_message"`
	Metadata             string     `json:"metadata" db:"metadata"` // JSON string
	CreatedAt            time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at" db:"updated_at"`
}

// AIAnalysisAudit represents AI analysis audit data
// BR-STORAGE-002: Audit trail for AI analysis
type AIAnalysisAudit struct {
	ID                   int64     `json:"id" db:"id"`
	RemediationRequestID string    `json:"remediation_request_id" db:"remediation_request_id"`
	AnalysisID           string    `json:"analysis_id" db:"analysis_id"`
	Provider             string    `json:"provider" db:"provider"` // holmesgpt, openai, claude
	Model                string    `json:"model" db:"model"`
	ConfidenceScore      float64   `json:"confidence_score" db:"confidence_score"`
	TokensUsed           int       `json:"tokens_used" db:"tokens_used"`
	AnalysisDuration     int64     `json:"analysis_duration" db:"analysis_duration"` // milliseconds
	Metadata             string    `json:"metadata" db:"metadata"`                   // JSON string
	CreatedAt            time.Time `json:"created_at" db:"created_at"`
}

// WorkflowAudit represents workflow execution audit data
// BR-STORAGE-003: Audit trail for workflow execution
type WorkflowAudit struct {
	ID                   int64      `json:"id" db:"id"`
	RemediationRequestID string     `json:"remediation_request_id" db:"remediation_request_id"`
	WorkflowID           string     `json:"workflow_id" db:"workflow_id"`
	Phase                string     `json:"phase" db:"phase"` // planning, executing, completed, failed
	TotalSteps           int        `json:"total_steps" db:"total_steps"`
	CompletedSteps       int        `json:"completed_steps" db:"completed_steps"`
	StartTime            time.Time  `json:"start_time" db:"start_time"`
	EndTime              *time.Time `json:"end_time,omitempty" db:"end_time"`
	Metadata             string     `json:"metadata" db:"metadata"` // JSON string
	CreatedAt            time.Time  `json:"created_at" db:"created_at"`
}

// ExecutionAudit represents action execution audit data
// BR-STORAGE-004: Audit trail for action execution
type ExecutionAudit struct {
	ID             int64      `json:"id" db:"id"`
	WorkflowID     string     `json:"workflow_id" db:"workflow_id"`
	ExecutionID    string     `json:"execution_id" db:"execution_id"`
	ActionType     string     `json:"action_type" db:"action_type"`
	TargetResource string     `json:"target_resource" db:"target_resource"`
	ClusterName    string     `json:"cluster_name" db:"cluster_name"`
	Success        bool       `json:"success" db:"success"`
	StartTime      time.Time  `json:"start_time" db:"start_time"`
	EndTime        *time.Time `json:"end_time,omitempty" db:"end_time"`
	ExecutionTime  int64      `json:"execution_time" db:"execution_time"` // milliseconds
	ErrorMessage   *string    `json:"error_message,omitempty" db:"error_message"`
	Metadata       string     `json:"metadata" db:"metadata"` // JSON string
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
}
