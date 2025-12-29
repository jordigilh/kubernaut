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

package notification

import (
	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
	notificationaudit "github.com/jordigilh/kubernaut/pkg/notification/audit"
)

// ========================================
// AUDIT MANAGER WRAPPER (P3 PATTERN)
// 📋 Controller Refactoring Pattern: Audit Manager
// ========================================
//
// This file provides a thin wrapper around pkg/notification/audit.Manager
// for backwards compatibility with existing controller code.
//
// PATTERN ADOPTION (December 28, 2025):
// - Audit logic extracted to pkg/notification/audit/manager.go
// - Controller imports from pkg for reusability
// - Maintains API compatibility for existing code
//
// See: pkg/notification/audit/manager.go for implementation
// ========================================

// AuditHelpers provides helper functions for creating notification audit events
// following ADR-034 unified audit table format.
//
// This is a thin wrapper around pkg/notification/audit.Manager for backwards
// compatibility. New code should use audit.Manager directly.
//
// BR-NOT-062: Unified Audit Table Integration
// BR-NOT-063: Graceful Audit Degradation
// BR-NOT-064: Audit Event Correlation
//
// See: docs/services/crd-controllers/06-notification/DD-NOT-001-ADR034-AUDIT-INTEGRATION-v2.0-FULL.md
type AuditHelpers struct {
	manager *notificationaudit.Manager
}

// NewAuditHelpers creates a new AuditHelpers instance
//
// This wraps pkg/notification/audit.NewManager() for backwards compatibility.
//
// Parameters:
//   - serviceName: Name of the service for actor_id field (e.g., "notification-controller")
func NewAuditHelpers(serviceName string) *AuditHelpers {
	return &AuditHelpers{
		manager: notificationaudit.NewManager(serviceName),
	}
}

// CreateMessageSentEvent creates an audit event for successful message delivery
//
// Delegates to pkg/notification/audit.Manager.CreateMessageSentEvent()
func (a *AuditHelpers) CreateMessageSentEvent(notification *notificationv1alpha1.NotificationRequest, channel string) (*dsgen.AuditEventRequest, error) {
	return a.manager.CreateMessageSentEvent(notification, channel)
}

// CreateMessageFailedEvent creates an audit event for failed message delivery
//
// Delegates to pkg/notification/audit.Manager.CreateMessageFailedEvent()
func (a *AuditHelpers) CreateMessageFailedEvent(notification *notificationv1alpha1.NotificationRequest, channel string, err error) (*dsgen.AuditEventRequest, error) {
	return a.manager.CreateMessageFailedEvent(notification, channel, err)
}

// CreateMessageAcknowledgedEvent creates an audit event for acknowledged notification
//
// Delegates to pkg/notification/audit.Manager.CreateMessageAcknowledgedEvent()
func (a *AuditHelpers) CreateMessageAcknowledgedEvent(notification *notificationv1alpha1.NotificationRequest) (*dsgen.AuditEventRequest, error) {
	return a.manager.CreateMessageAcknowledgedEvent(notification)
}

// CreateMessageEscalatedEvent creates an audit event for escalated notification
//
// Delegates to pkg/notification/audit.Manager.CreateMessageEscalatedEvent()
func (a *AuditHelpers) CreateMessageEscalatedEvent(notification *notificationv1alpha1.NotificationRequest) (*dsgen.AuditEventRequest, error) {
	return a.manager.CreateMessageEscalatedEvent(notification)
}
