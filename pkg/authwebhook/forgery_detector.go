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

package authwebhook

import (
	"github.com/go-logr/logr"
)

// REFACTOR-AW-002: Identity forgery detection extracted for SOC 2 compliance tracking
// Reference: BR-AUTH-001 (SOC 2 CC8.1 User Attribution), CC6.8 (Non-Repudiation)

// DetectAndLogForgeryAttempt checks if a user-provided DecidedBy field exists
// and logs a security warning if identity forgery is attempted.
//
// Per BR-AUTH-001, SOC 2 CC8.1: User attribution MUST be tamper-proof.
// This function provides forensic evidence of forgery attempts.
//
// Returns true if forgery was detected (user provided DecidedBy), false otherwise.
func DetectAndLogForgeryAttempt(logger logr.Logger, userProvidedDecidedBy, authenticatedUser string) bool {
	if userProvidedDecidedBy == "" {
		// No forgery - user did not attempt to set DecidedBy
		return false
	}

	// SECURITY: Log forgery attempt for SOC 2 compliance and forensics
	logger.Info("SECURITY: Overwriting user-provided DecidedBy (forgery prevention)",
		"userProvidedValue", userProvidedDecidedBy,
		"authenticatedUser", authenticatedUser,
	)

	return true
}
