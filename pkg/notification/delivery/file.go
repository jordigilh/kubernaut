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
	"sigs.k8s.io/yaml"
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

// Ensure FileDeliveryService implements Service interface at compile-time
var _ Service = (*FileDeliveryService)(nil)

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
//   - Filename: notification-{name}-{timestamp}.{format}
//   - Content: Complete NotificationRequest as JSON or YAML (pretty-printed)
//   - Timestamp: Microsecond precision to prevent collisions
//
// TDD GREEN Enhancement:
//   - Uses FileDeliveryConfig from CRD if specified
//   - Falls back to constructor outputDir if FileDeliveryConfig not specified
//   - Supports JSON and YAML formats
//
// Error Handling (V3.0):
//   - Directory creation failures: Return error (test environment issue)
//   - JSON/YAML marshaling failures: Return error (should never happen)
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

	// Use service-level configuration (constructor outputDir)
	// Per design decision: Channel-specific config should NOT be in CRD
	outputDir := s.outputDir
	format := "json" // Default format (hardcoded for E2E simplicity)

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Error(err, "Failed to create file output directory",
			"outputDir", outputDir,
			"notification", notification.Name,
			"namespace", notification.Namespace)
		// NT-BUG-006: Wrap directory creation errors as retryable (permission denied, disk full, etc.)
		// These are temporary errors that may resolve after directory permissions are fixed
		// Fixes Test 06 (Multi-Channel Fanout) - ensures directory permission errors trigger retry
		return NewRetryableError(fmt.Errorf("failed to create output directory: %w", err))
	}

	// Generate filename with timestamp and format (prevents overwrites)
	filename := s.generateFilenameWithFormat(notification, format)
	filePath := filepath.Join(outputDir, filename)

	log.Info("Delivering notification to file",
		"notification", notification.Name,
		"namespace", notification.Namespace,
		"filename", filename,
		"outputDir", outputDir,
		"format", format)

	// TDD REFACTOR: Marshal notification to specified format (pretty-printed for readability)
	var data []byte
	var err error

	switch format {
	case "json":
		data, err = json.MarshalIndent(notification, "", "  ")
		if err != nil {
			log.Error(err, "Failed to marshal notification to JSON",
				"notification", notification.Name,
				"namespace", notification.Namespace)
			return fmt.Errorf("failed to marshal notification to JSON: %w", err)
		}
	case "yaml":
		// TDD REFACTOR: YAML support added
		data, err = yaml.Marshal(notification)
		if err != nil {
			log.Error(err, "Failed to marshal notification to YAML",
				"notification", notification.Name,
				"namespace", notification.Namespace)
			return fmt.Errorf("failed to marshal notification to YAML: %w", err)
		}
	default:
		return fmt.Errorf("unsupported file format: %s (supported: json, yaml)", format)
	}

	// TDD REFACTOR: Atomic file write (write to temp, then rename)
	// This prevents partial writes if the process crashes during write
	tempFile := filePath + ".tmp"

	// Write to temporary file first
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		log.Error(err, "Failed to write temporary notification file",
			"notification", notification.Name,
			"namespace", notification.Namespace,
			"tempFile", tempFile)
		// NT-BUG-006: Wrap file system errors as retryable (permission denied, disk full, etc.)
		// These are temporary errors that may resolve after directory permissions are fixed
		return NewRetryableError(fmt.Errorf("failed to write temporary file: %w", err))
	}

	// Atomically rename temp file to final file
	if err := os.Rename(tempFile, filePath); err != nil {
		log.Error(err, "Failed to rename temporary file to final file",
			"notification", notification.Name,
			"namespace", notification.Namespace,
			"tempFile", tempFile,
			"filePath", filePath)
		// Clean up temp file on error
		if removeErr := os.Remove(tempFile); removeErr != nil {
			log.Error(removeErr, "Failed to remove temporary file after rename error",
				"tempFile", tempFile)
		}
		// NT-BUG-006: Wrap rename errors as retryable (permission denied, etc.)
		return NewRetryableError(fmt.Errorf("failed to rename temporary file: %w", err))
	}

	log.Info("Notification delivered successfully to file",
		"notification", notification.Name,
		"namespace", notification.Namespace,
		"filePath", filePath,
		"filesize", len(data),
		"format", format)

	return nil
}

// generateFilename creates a unique filename for the notification (legacy method).
//
// Format: notification-{name}-{timestamp}.json
// Example: notification-critical-alert-20251123-143022.123456.json
//
// Timestamp includes microseconds to prevent collisions in high-throughput scenarios.
// This ensures thread-safe concurrent delivery without overwrites.
//
// DEPRECATED: Use generateFilenameWithFormat instead
func (s *FileDeliveryService) generateFilename(notification *notificationv1alpha1.NotificationRequest) string {
	timestamp := time.Now().Format("20060102-150405.000000")
	return fmt.Sprintf("notification-%s-%s.json", notification.Name, timestamp)
}

// generateFilenameWithFormat creates a unique filename for the notification with specified format.
//
// Format: notification-{name}-{timestamp}.{format}
// Example: notification-critical-alert-20251123-143022.123456.json
//
// TDD GREEN: Minimal implementation
// - Supports json and yaml formats
// - Timestamp includes microseconds to prevent collisions
//
// Parameters:
//   - notification: The notification request
//   - format: File format (json or yaml)
func (s *FileDeliveryService) generateFilenameWithFormat(notification *notificationv1alpha1.NotificationRequest, format string) string {
	timestamp := time.Now().Format("20060102-150405.000000")
	return fmt.Sprintf("notification-%s-%s.%s", notification.Name, timestamp, format)
}
