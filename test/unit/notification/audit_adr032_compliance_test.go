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
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	notificationcontroller "github.com/jordigilh/kubernaut/internal/controller/notification"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	notificationaudit "github.com/jordigilh/kubernaut/pkg/notification/audit"
)

// ========================================
// ADR-032 §1 Compliance Tests - NEGATIVE TESTS
// ========================================
//
// Business Requirements:
// - ADR-032 §1: Audit is MANDATORY - no graceful degradation allowed
// - ADR-032 §2: No Recovery Allowed - fail fast if audit unavailable
// - ADR-032 §4: Enforcement - services MUST return error if audit store is nil
//
// Test Strategy: These are NEGATIVE TESTS that verify failure behavior
// - Validate that audit functions return errors when store is nil
// - Validate that error messages cite ADR-032 §1
// - Validate that no silent audit loss occurs
//
// Expected Results:
// - All audit functions return error when AuditStore is nil
// - All audit functions return error when AuditHelpers is nil
// - Error messages explicitly mention ADR-032 §1
// - No graceful degradation or silent skip behavior
//
// Note: These tests do NOT validate controller initialization crash behavior
//       (that's tested at integration/E2E level with real pod startup)

var _ = Describe("ADR-032 §1 Compliance Tests", Label("unit", "audit", "adr-032"), func() {
	var (
		ctx          context.Context
		notification *notificationv1alpha1.NotificationRequest
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create test notification
		notification = &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-notification",
				Namespace: "default",
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Subject:  "Test Notification",
				Body:     "Test body",
				Priority: "critical",
				Channels: []notificationv1alpha1.Channel{
					notificationv1alpha1.ChannelSlack,
				},
				// BR-NOT-064: Correlation ID from Spec.Metadata (not Labels)
				Metadata: map[string]string{
					"remediationRequestName": "test-remediation-123",
				},
			},
		}
	})

	// ========================================
	// NEGATIVE TEST 1: auditMessageSent with nil AuditStore
	// ========================================
	Describe("auditMessageSent ADR-032 §1 Compliance", func() {
		It("MUST return error when AuditStore is nil", func() {
			// GIVEN: Reconciler with nil AuditStore
			reconciler := &notificationcontroller.NotificationRequestReconciler{
				AuditStore:   nil, // ❌ VIOLATION SCENARIO
				AuditManager: notificationaudit.NewManager("test-service"),
			}

			// WHEN: Attempting to audit message sent
			err := reconciler.ExportedAuditMessageSent(ctx, notification, "slack")

			// THEN: MUST return error (not silent skip)
			Expect(err).To(HaveOccurred(), "ADR-032 §1: MUST fail when audit store is nil")
			Expect(err.Error()).To(ContainSubstring("ADR-032 §1"),
				"Error message MUST cite ADR-032 §1")
			Expect(err.Error()).To(ContainSubstring("MANDATORY"),
				"Error message MUST indicate audit is mandatory")
		})

		It("MUST return error when AuditHelpers is nil", func() {
			// GIVEN: Reconciler with nil AuditHelpers
			reconciler := &notificationcontroller.NotificationRequestReconciler{
				AuditStore:   &mockAuditStore{}, // Store exists
				AuditManager: nil,               // ❌ VIOLATION SCENARIO
			}

			// WHEN: Attempting to audit message sent
			err := reconciler.ExportedAuditMessageSent(ctx, notification, "slack")

			// THEN: MUST return error (not silent skip)
			Expect(err).To(HaveOccurred(), "ADR-032 §1: MUST fail when audit helpers is nil")
			Expect(err.Error()).To(ContainSubstring("ADR-032 §1"),
				"Error message MUST cite ADR-032 §1")
		})
	})

	// ========================================
	// NEGATIVE TEST 2: auditMessageFailed with nil AuditStore
	// ========================================
	Describe("auditMessageFailed ADR-032 §1 Compliance", func() {
		It("MUST return error when AuditStore is nil", func() {
			// GIVEN: Reconciler with nil AuditStore
			reconciler := &notificationcontroller.NotificationRequestReconciler{
				AuditStore:   nil, // ❌ VIOLATION SCENARIO
				AuditManager: notificationaudit.NewManager("test-service"),
			}

			deliveryErr := fmt.Errorf("simulated delivery failure")

			// WHEN: Attempting to audit message failed
			err := reconciler.ExportedAuditMessageFailed(ctx, notification, "slack", deliveryErr)

			// THEN: MUST return error (not silent skip)
			Expect(err).To(HaveOccurred(), "ADR-032 §1: MUST fail when audit store is nil")
			Expect(err.Error()).To(ContainSubstring("ADR-032 §1"),
				"Error message MUST cite ADR-032 §1")
			Expect(err.Error()).To(ContainSubstring("MANDATORY"),
				"Error message MUST indicate audit is mandatory")
		})

		It("MUST return error when AuditHelpers is nil", func() {
			// GIVEN: Reconciler with nil AuditHelpers
			reconciler := &notificationcontroller.NotificationRequestReconciler{
				AuditStore:   &mockAuditStore{}, // Store exists
				AuditManager: nil,               // ❌ VIOLATION SCENARIO
			}

			deliveryErr := fmt.Errorf("simulated delivery failure")

			// WHEN: Attempting to audit message failed
			err := reconciler.ExportedAuditMessageFailed(ctx, notification, "slack", deliveryErr)

			// THEN: MUST return error (not silent skip)
			Expect(err).To(HaveOccurred(), "ADR-032 §1: MUST fail when audit helpers is nil")
			Expect(err.Error()).To(ContainSubstring("ADR-032 §1"),
				"Error message MUST cite ADR-032 §1")
		})
	})

	// ========================================
	// NEGATIVE TEST 3: auditMessageAcknowledged with nil AuditStore
	// ========================================
	Describe("auditMessageAcknowledged ADR-032 §1 Compliance", func() {
		It("MUST return error when AuditStore is nil", func() {
			// GIVEN: Reconciler with nil AuditStore
			reconciler := &notificationcontroller.NotificationRequestReconciler{
				AuditStore:   nil, // ❌ VIOLATION SCENARIO
				AuditManager: notificationaudit.NewManager("test-service"),
			}

			// WHEN: Attempting to audit message acknowledged
			err := reconciler.ExportedAuditMessageAcknowledged(ctx, notification)

			// THEN: MUST return error (not silent skip)
			Expect(err).To(HaveOccurred(), "ADR-032 §1: MUST fail when audit store is nil")
			Expect(err.Error()).To(ContainSubstring("ADR-032 §1"),
				"Error message MUST cite ADR-032 §1")
		})

		It("MUST return error when AuditHelpers is nil", func() {
			// GIVEN: Reconciler with nil AuditHelpers
			reconciler := &notificationcontroller.NotificationRequestReconciler{
				AuditStore:   &mockAuditStore{}, // Store exists
				AuditManager: nil,               // ❌ VIOLATION SCENARIO
			}

			// WHEN: Attempting to audit message acknowledged
			err := reconciler.ExportedAuditMessageAcknowledged(ctx, notification)

			// THEN: MUST return error (not silent skip)
			Expect(err).To(HaveOccurred(), "ADR-032 §1: MUST fail when audit helpers is nil")
			Expect(err.Error()).To(ContainSubstring("ADR-032 §1"),
				"Error message MUST cite ADR-032 §1")
		})
	})

	// ========================================
	// NEGATIVE TEST 4: auditMessageEscalated with nil AuditStore
	// ========================================
	Describe("auditMessageEscalated ADR-032 §1 Compliance", func() {
		It("MUST return error when AuditStore is nil", func() {
			// GIVEN: Reconciler with nil AuditStore
			reconciler := &notificationcontroller.NotificationRequestReconciler{
				AuditStore:   nil, // ❌ VIOLATION SCENARIO
				AuditManager: notificationaudit.NewManager("test-service"),
			}

			// WHEN: Attempting to audit message escalated
			err := reconciler.ExportedAuditMessageEscalated(ctx, notification)

			// THEN: MUST return error (not silent skip)
			Expect(err).To(HaveOccurred(), "ADR-032 §1: MUST fail when audit store is nil")
			Expect(err.Error()).To(ContainSubstring("ADR-032 §1"),
				"Error message MUST cite ADR-032 §1")
		})

		It("MUST return error when AuditHelpers is nil", func() {
			// GIVEN: Reconciler with nil AuditHelpers
			reconciler := &notificationcontroller.NotificationRequestReconciler{
				AuditStore:   &mockAuditStore{}, // Store exists
				AuditManager: nil,               // ❌ VIOLATION SCENARIO
			}

			// WHEN: Attempting to audit message escalated
			err := reconciler.ExportedAuditMessageEscalated(ctx, notification)

			// THEN: MUST return error (not silent skip)
			Expect(err).To(HaveOccurred(), "ADR-032 §1: MUST fail when audit helpers is nil")
			Expect(err.Error()).To(ContainSubstring("ADR-032 §1"),
				"Error message MUST cite ADR-032 §1")
		})
	})

	// ========================================
	// POSITIVE TEST: Verify error is NOT returned when audit succeeds
	// ========================================
	Describe("ADR-032 §1 Compliance - Success Path", func() {
		It("SHOULD NOT return error when audit store is properly initialized", func() {
			// GIVEN: Reconciler with valid AuditStore and AuditHelpers
			reconciler := &notificationcontroller.NotificationRequestReconciler{
				AuditStore:   &mockAuditStore{}, // ✅ Valid store
				AuditManager: notificationaudit.NewManager("test-service"),
			}

			// WHEN: Attempting to audit message sent
			err := reconciler.ExportedAuditMessageSent(ctx, notification, "slack")

			// THEN: SHOULD succeed (no error)
			Expect(err).ToNot(HaveOccurred(),
				"ADR-032 §1: Should succeed when audit store is properly initialized")
		})
	})
})

// ========================================
// Mock Audit Store (Minimal Implementation)
// ========================================
// Used for testing audit function behavior without real DataStorage dependency

type mockAuditStore struct{}

func (m *mockAuditStore) StoreAudit(_ context.Context, _ *ogenclient.AuditEventRequest) error {
	return nil // Success case for positive tests
}

func (m *mockAuditStore) Flush(_ context.Context) error {
	return nil
}

func (m *mockAuditStore) Close() error {
	return nil
}
