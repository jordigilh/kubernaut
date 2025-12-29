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

package audit

import (
	"context"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// Writer defines the interface for writing audit records to the Data Storage Service
// This interface abstracts the audit write operations, allowing for:
// - Easy mocking in tests
// - Multiple implementations (HTTP client, direct DB, mock)
// - Consistent audit write patterns across all controllers
//
// Authority: ADR-032 v1.3 - Data Access Layer Isolation
// Pattern: All controllers use this interface to write audit data
type Writer interface {
	// WriteNotificationAudit writes a notification audit record
	// This is called by the Notification Controller after each CRD status update
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - audit: NotificationAudit record to persist
	//
	// Returns:
	//   - error: nil on success, error on failure (triggers DLQ fallback per DD-009)
	//
	// Behavior:
	//   - Non-blocking: Should be called in a goroutine by controllers
	//   - DLQ Fallback: On failure, audit data is pushed to Redis DLQ (DD-009)
	//   - Idempotent: Safe to retry (notification_id is UNIQUE constraint)
	//
	// Example:
	//   auditData := &models.NotificationAudit{...}
	//   go func() {
	//       if err := writer.WriteNotificationAudit(ctx, auditData); err != nil {
	//           log.Error(err, "Failed to write audit (DLQ fallback triggered)")
	//       }
	//   }()
	WriteNotificationAudit(ctx context.Context, audit *models.NotificationAudit) error

	// Future audit write methods will be added here as controllers are implemented:
	// - WriteSignalProcessingAudit(ctx context.Context, audit *models.SignalProcessingAudit) error
	// - WriteOrchestrationAudit(ctx context.Context, audit *models.OrchestrationAudit) error
	// - WriteAIAnalysisAudit(ctx context.Context, audit *models.AIAnalysisAudit) error
	// - WriteWorkflowExecutionAudit(ctx context.Context, audit *models.WorkflowExecutionAudit) error
	// - WriteEffectivenessAudit(ctx context.Context, audit *models.EffectivenessAudit) error
}

// Reader defines the interface for reading audit records from the Data Storage Service
// This interface will be implemented in Phase 2 when read operations are needed
//
// Authority: ADR-032 v1.3 - Data Access Layer Isolation
// Status: DEFERRED - Read API not needed for Phase 1 (write-only)
type Reader interface {
	// Future read methods will be added here:
	// - GetNotificationAudit(ctx context.Context, notificationID string) (*models.NotificationAudit, error)
	// - ListNotificationAudits(ctx context.Context, filters AuditFilters) ([]*models.NotificationAudit, error)
}

// Repository defines the complete interface for audit data access
// This combines Writer and Reader for services that need both operations
//
// Authority: ADR-032 v1.3 - Data Access Layer Isolation
// Status: Phase 1 implements Writer only, Reader deferred to Phase 2
type Repository interface {
	Writer
	Reader
}
