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

package delivery

import (
	"context"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// ========================================
// DELIVERY SERVICE INTERFACE (DD-NOT-002 V3.0)
// 📋 Design Decision: DD-NOT-002 (File-Based E2E Tests)
// ✅ Interface-First Approach | Confidence: 95%
// See: docs/services/crd-controllers/06-notification/DD-NOT-002-FILE-BASED-E2E-TESTS_IMPLEMENTATION_PLAN_V3.0.md
// ========================================
//
// DeliveryService defines the common interface for all notification delivery mechanisms.
//
// All delivery services (Console, Slack, File, future: Email, PagerDuty, Teams, etc.)
// implement this interface for polymorphic usage in the notification controller.
//
// WHY THIS INTERFACE? (DD-NOT-002 V3.0 - Interface-First Approach)
// - ✅ Clean Architecture: Defines contract before implementation
// - ✅ Future-Proof: Easy to add new delivery services
// - ✅ Type Safety: Compile-time validation of interface compliance
// - ✅ Testing: Enables mocking for controller tests
//
// DESIGN DECISION: Interface-First (Concern #1 from Pre-Implementation Triage)
// - Prevents breaking changes to existing services
// - FileDeliveryService implements this from day 1
// - Existing services (Console, Slack) remain as concrete types (acceptable technical debt)
//
// Business Requirements:
// - BR-NOT-053: At-Least-Once Delivery (all services must deliver notifications)
// - BR-NOT-054: Data Sanitization (delivery happens after sanitization in controller)
// - BR-NOT-056: Priority-Based Routing (services respect priority from notification)
//
// ========================================
type DeliveryService interface {
	// Deliver sends a notification through this delivery mechanism.
	//
	// The notification is delivered as-is; sanitization should be applied
	// by the controller BEFORE calling Deliver.
	//
	// Context is provided for cancellation and timeouts.
	//
	// Returns:
	//   - nil if delivery succeeds
	//   - error if delivery fails (caller decides whether to retry)
	//
	// Implementation Requirements:
	//   - MUST respect context cancellation
	//   - MUST return descriptive errors
	//   - SHOULD be idempotent (safe to retry)
	//   - SHOULD log delivery attempts (structured logging)
	Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error
}
