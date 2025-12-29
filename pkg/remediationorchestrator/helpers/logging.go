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

// Package helpers provides common helper utilities for Remediation Orchestrator.
// Reference: REFACTOR-RO-006
package helpers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ========================================
// LOGGING HELPERS (REFACTOR-RO-006)
// ========================================
//
// Centralized logging patterns to ensure consistency across RO controllers and handlers.
//
// WHY Extract Logging Patterns?
// - ✅ Consistent logging format across all RO components
// - ✅ Reduced boilerplate in handler methods
// - ✅ Easier to add structured fields (e.g., tracing IDs)
// - ✅ Single place to enforce logging standards
//
// Reference: REFACTOR-RO-006 (logging pattern extraction)

// WithMethodLogging creates a logger with method-level context and logs entry/exit.
//
// Usage:
//
//	logger := helpers.WithMethodLogging(ctx, "HandleSkipped", "remediationRequest", rr.Name)
//	defer logger.V(1).Info("Method complete")
//
// This pattern ensures:
// - Consistent method name logging
// - Entry/exit visibility for debugging
// - Structured context fields
//
// Reference: REFACTOR-RO-006
func WithMethodLogging(
	ctx context.Context,
	methodName string,
	keysAndValues ...interface{},
) logr.Logger {
	logger := log.FromContext(ctx).WithValues(append([]interface{}{"method", methodName}, keysAndValues...)...)
	logger.V(1).Info("Method entry")
	return logger
}

// LogAndWrapError logs an error with additional context and wraps it with a message.
//
// Usage:
//
//	if err := doSomething(); err != nil {
//	    return helpers.LogAndWrapError(logger, err, "Failed to do something")
//	}
//
// This pattern ensures:
// - Error is logged before being returned
// - Error is wrapped with additional context
// - Consistent error logging format
//
// Reference: REFACTOR-RO-006
func LogAndWrapError(logger logr.Logger, err error, message string) error {
	logger.Error(err, message)
	return fmt.Errorf("%s: %w", message, err)
}

// LogAndWrapErrorf logs an error with formatted message and wraps it.
//
// Usage:
//
//	if err := doSomething(); err != nil {
//	    return helpers.LogAndWrapErrorf(logger, err, "Failed to process RR %s", rr.Name)
//	}
//
// Reference: REFACTOR-RO-006
func LogAndWrapErrorf(logger logr.Logger, err error, format string, args ...interface{}) error {
	message := fmt.Sprintf(format, args...)
	logger.Error(err, message)
	return fmt.Errorf("%s: %w", message, err)
}

// LogInfo logs an info message with consistent formatting.
//
// Usage:
//
//	helpers.LogInfo(logger, "Processing remediation", "phase", rr.Status.OverallPhase)
//
// Reference: REFACTOR-RO-006
func LogInfo(logger logr.Logger, message string, keysAndValues ...interface{}) {
	logger.Info(message, keysAndValues...)
}

// LogInfoV logs a verbose info message (for debugging).
//
// Usage:
//
//	helpers.LogInfoV(logger, 1, "Detailed debug info", "details", debugData)
//
// Reference: REFACTOR-RO-006
func LogInfoV(logger logr.Logger, level int, message string, keysAndValues ...interface{}) {
	logger.V(level).Info(message, keysAndValues...)
}

// LogError logs an error with consistent formatting.
//
// Usage:
//
//	helpers.LogError(logger, err, "Failed to update status", "rr", rr.Name)
//
// Reference: REFACTOR-RO-006
func LogError(logger logr.Logger, err error, message string, keysAndValues ...interface{}) {
	logger.Error(err, message, keysAndValues...)
}
