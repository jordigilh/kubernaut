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
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ErrInvalidTimeoutConfig is a sentinel error for invalid timeout configuration.
// Callers should wrap this error instead of embedding "ERR_INVALID_TIMEOUT_CONFIG"
// in error message strings.
//
// Usage:
//
//	return fmt.Errorf("negative global timeout: %w", audit.ErrInvalidTimeoutConfig)
var ErrInvalidTimeoutConfig = errors.New("ERR_INVALID_TIMEOUT_CONFIG")

// ErrorClassification holds the classified error code and retry guidance.
type ErrorClassification struct {
	Code          string
	RetryPossible bool
}

// ClassifyError classifies an error into a standardized error code with retry guidance.
//
// Classification priority (most specific first):
//  1. context.DeadlineExceeded → ERR_TIMEOUT_REMEDIATION
//  2. ErrInvalidTimeoutConfig sentinel → ERR_INVALID_TIMEOUT_CONFIG
//  3. apierrors.IsNotFound → ERR_K8S_NOT_FOUND
//  4. apierrors.IsAlreadyExists → ERR_K8S_ALREADY_EXISTS
//  5. apierrors.IsConflict → ERR_K8S_CONFLICT
//  6. apierrors.IsForbidden → ERR_K8S_FORBIDDEN
//  7. apierrors.IsTimeout → ERR_TIMEOUT_REMEDIATION
//  8. apierrors.IsInvalid → ERR_INVALID_CONFIG
//  9. apierrors.IsServiceUnavailable → ERR_K8S_SERVICE_UNAVAILABLE
//  10. Default → ERR_INTERNAL_ORCHESTRATION
func ClassifyError(err error) ErrorClassification {
	if err == nil {
		return ErrorClassification{Code: "ERR_INTERNAL_ORCHESTRATION", RetryPossible: true}
	}

	// 1. context.DeadlineExceeded (check before K8s timeout — more specific)
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorClassification{Code: "ERR_TIMEOUT_REMEDIATION", RetryPossible: true}
	}

	// 2. Sentinel: ErrInvalidTimeoutConfig
	if errors.Is(err, ErrInvalidTimeoutConfig) {
		return ErrorClassification{Code: "ERR_INVALID_TIMEOUT_CONFIG", RetryPossible: false}
	}

	// 3-9. Kubernetes API errors (apierrors.Is* functions unwrap automatically)
	switch {
	case apierrors.IsNotFound(err):
		return ErrorClassification{Code: "ERR_K8S_NOT_FOUND", RetryPossible: true}
	case apierrors.IsAlreadyExists(err):
		return ErrorClassification{Code: "ERR_K8S_ALREADY_EXISTS", RetryPossible: false}
	case apierrors.IsConflict(err):
		return ErrorClassification{Code: "ERR_K8S_CONFLICT", RetryPossible: true}
	case apierrors.IsForbidden(err):
		return ErrorClassification{Code: "ERR_K8S_FORBIDDEN", RetryPossible: false}
	case apierrors.IsTimeout(err):
		return ErrorClassification{Code: "ERR_TIMEOUT_REMEDIATION", RetryPossible: true}
	case apierrors.IsInvalid(err):
		return ErrorClassification{Code: "ERR_INVALID_CONFIG", RetryPossible: false}
	case apierrors.IsServiceUnavailable(err):
		return ErrorClassification{Code: "ERR_K8S_SERVICE_UNAVAILABLE", RetryPossible: true}
	}

	// 10. Default
	return ErrorClassification{Code: "ERR_INTERNAL_ORCHESTRATION", RetryPossible: true}
}
