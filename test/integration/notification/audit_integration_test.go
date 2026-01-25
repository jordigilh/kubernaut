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

// ========================================
// NOTIFICATION AUDIT INTEGRATION TESTS - DELETED (December 26, 2025)
// ========================================
//
// **STATUS**: All 6 tests from this file have been DELETED per anti-pattern triage.
//
// **WHY DELETED**:
// These tests followed the WRONG PATTERN: They directly called audit store methods
// (auditStore.StoreAudit()) to test audit infrastructure, NOT Notification controller behavior.
//
// **What they tested** (audit client library):
// - ✅ Audit client buffering works
// - ✅ Audit client batching works
// - ✅ Audit client graceful shutdown works
// - ✅ Audit client correlation works
// - ✅ Audit client ADR-034 compliance works
//
// **What they did NOT test** (Notification controller):
// - ❌ Notification controller emits audits during delivery
// - ❌ Notification controller integrates audit calls into business flows
// - ❌ Audit events are emitted at the right time in the business flow
//
// ========================================
// MIGRATION
// ========================================
//
// **Audit Infrastructure Tests** → Should be in:
// - pkg/audit/buffered_store_integration_test.go (audit client library tests)
// - test/integration/datastorage/audit_events_*_test.go (DataStorage service tests)
//
// **Notification Controller Audit Tests** → Should be created following CORRECT PATTERN:
//
// CORRECT PATTERN (see SignalProcessing/Gateway as examples):
// 1. Create NotificationRequest CRD (trigger business logic)
// 2. Wait for controller to process (business logic execution)
// 3. Verify audit event was emitted (side effect validation)
//
// Example:
// ```go
// It("should emit notification.message.sent audit when delivery succeeds", func() {
//     // 1. Trigger business logic
//     notif := &notificationv1alpha1.NotificationRequest{...}
//     k8sClient.Create(ctx, notif)
//
//     // 2. Wait for controller to deliver
//     Eventually(func() Phase {
//         var updated NotificationRequest
//         k8sManager.GetAPIReader().Get(ctx, ..., &updated)
//         return updated.Status.Phase
//     }).Should(Equal(NotificationPhaseSent))
//
//     // 3. Verify controller emitted audit event
//     Eventually(func() int {
//         resp, _ := dsClient.QueryAuditEventsWithResponse(ctx, &ogenclient.QueryAuditEventsParams{
//             EventType:     ptr.To("notification.message.sent"),
//             CorrelationId: &notif.Name,
//         })
//         return *resp.JSON200.Pagination.Total
//     }).Should(Equal(1))
// })
// ```
//
// ========================================
// REFERENCES
// ========================================
//
// **Authoritative Documentation**:
// - TESTING_GUIDELINES.md v2.5.0 (lines 1679-1900+)
//   Section: "🚫 ANTI-PATTERN: Direct Audit Infrastructure Testing"
//
// **Triage Document**:
// - docs/handoff/AUDIT_INFRASTRUCTURE_TESTING_ANTI_PATTERN_TRIAGE_DEC_26_2025.md
//
// **Correct Pattern Examples**:
// - test/integration/signalprocessing/audit_integration_test.go (lines 97-196)
// - test/integration/gateway/audit_integration_test.go (lines 171-226)
//
// **Tracking Issue**: Create issue for flow-based audit tests implementation
//
// ========================================
// DELETED TESTS (6 total)
// ========================================
//
// 1. "should write audit event to Data Storage Service and be queryable via REST API" (BR-NOT-062)
// 2. "should flush batch of events and be queryable via REST API" (BR-NOT-062)
// 3. "should not block when storing audit events (fire-and-forget pattern)" (BR-NOT-063)
// 4. "should flush all remaining events before shutdown" (Graceful Shutdown)
// 5. "should enable workflow tracing via correlation_id" (BR-NOT-064)
// 6. "should persist event with all ADR-034 required fields" (ADR-034)
//
// All tests manually created audit events and called auditStore.StoreAudit().
// These tests belonged in pkg/audit or DataStorage service, not Notification.
//
// ========================================
// NEXT STEPS
// ========================================
//
// 1. Create tracking issue: "Implement flow-based audit tests for Notification controller"
// 2. Implement 2-3 flow-based tests:
//    - notification.message.sent (on successful delivery)
//    - notification.message.failed (on failed delivery)
//    - notification.message.acknowledged (on acknowledgment)
// 3. Use SignalProcessing/Gateway as reference implementations
//
// ========================================

// This file is intentionally empty - all tests have been deleted.
// See comments above for migration path and correct pattern.
