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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// ========================================
// FILE DELIVERY SERVICE (DD-NOT-002 V3.0)
// 📋 Design Decision: DD-NOT-002 (File-Based E2E Notification Delivery Validation)
// ✅ E2E Testing Infrastructure | Confidence: 95%
// See: docs/services/crd-controllers/06-notification/DD-NOT-002-FILE-BASED-E2E-TESTS_IMPLEMENTATION_PLAN_V3.0.md
// ========================================
//
// FileDeliveryService writes notification messages to JSON files for E2E test validation.
//
// PURPOSE: E2E Testing Infrastructure ONLY
// - This service is NOT used in production
// - Enables validation of complete notification message content
// - Provides ground truth for message delivery verification
//
// SAFETY GUARANTEE (V3.0 Error Handling Philosophy):
// - FileService failures MUST NOT block production notifications
// - Controller checks `if r.FileService != nil` before delivery
// - Errors are logged but NOT propagated to reconciliation
// - Production deployments have FileService = nil
//
// Business Requirements Validated by E2E Tests:
// - BR-NOT-053: At-Least-Once Delivery (file proves message delivered via controller)
// - BR-NOT-054: Data Sanitization (validates sanitization in controller flow)
// - BR-NOT-056: Priority-Based Routing (validates priority preserved via controller)
//
// ========================================

// FileDeliveryService writes notifications to JSON files for E2E testing.
//
// This service is E2E testing infrastructure only and should NOT be used in production.
// Files are written with timestamps to prevent overwrites in concurrent scenarios.
type FileDeliveryService struct {
	outputDir string // Directory where JSON files will be written
}

// Ensure FileDeliveryService implements DeliveryService interface at compile-time
var _ DeliveryService = (*FileDeliveryService)(nil)

// NewFileDeliveryService creates a new file-based delivery service for E2E testing.
//
// Parameters:
//   - outputDir: Directory where notification JSON files will be written
//
// The output directory will be created if it doesn't exist.
// Files are named: notification-{name}-{timestamp}.json
//
// Example:
//
//	service := NewFileDeliveryService("/tmp/kubernaut-e2e-notifications")
func NewFileDeliveryService(outputDir string) *FileDeliveryService {
	return &FileDeliveryService{
		outputDir: outputDir,
	}
}

// Deliver writes the notification to a JSON file for E2E test validation.
//
// File Format:
//   - Filename: notification-{name}-{timestamp}.json
//   - Content: Complete NotificationRequest as JSON (pretty-printed)
//   - Timestamp: Microsecond precision to prevent collisions
//
// Error Handling (V3.0):
//   - Directory creation failures: Return error (test environment issue)
//   - JSON marshaling failures: Return error (should never happen)
//   - File write failures: Return error (permissions, disk space, etc.)
//   - All errors are logged with structured logging
//
// Thread Safety:
//   - Safe for concurrent use (each notification gets unique filename)
//   - Timestamp includes microseconds to prevent collisions
//
// Implements: DeliveryService interface
func (s *FileDeliveryService) Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	log := ctrl.LoggerFrom(ctx)

	// Ensure output directory exists
	if err := os.MkdirAll(s.outputDir, 0755); err != nil {
		log.Error(err, "Failed to create file output directory",
			"outputDir", s.outputDir,
			"notification", notification.Name,
			"namespace", notification.Namespace)
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate filename with timestamp (prevents overwrites)
	filename := s.generateFilename(notification)
	filePath := filepath.Join(s.outputDir, filename)

	log.Info("Delivering notification to file",
		"notification", notification.Name,
		"namespace", notification.Namespace,
		"filename", filename,
		"outputDir", s.outputDir)

	// Marshal notification to JSON (pretty-printed for readability)
	data, err := json.MarshalIndent(notification, "", "  ")
	if err != nil {
		log.Error(err, "Failed to marshal notification to JSON",
			"notification", notification.Name,
			"namespace", notification.Namespace)
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		log.Error(err, "Failed to write notification file",
			"notification", notification.Name,
			"namespace", notification.Namespace,
			"filePath", filePath)
		return fmt.Errorf("failed to write notification file: %w", err)
	}

	log.Info("Notification delivered successfully to file",
		"notification", notification.Name,
		"namespace", notification.Namespace,
		"filePath", filePath,
		"filesize", len(data))

	return nil
}

// generateFilename creates a unique filename for the notification.
//
// Format: notification-{name}-{timestamp}.json
// Example: notification-critical-alert-20251123-143022.123456.json
//
// Timestamp includes microseconds to prevent collisions in high-throughput scenarios.
// This ensures thread-safe concurrent delivery without overwrites.
func (s *FileDeliveryService) generateFilename(notification *notificationv1alpha1.NotificationRequest) string {
	timestamp := time.Now().Format("20060102-150405.000000")
	return fmt.Sprintf("notification-%s-%s.json", notification.Name, timestamp)
}
