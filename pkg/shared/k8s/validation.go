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

package k8s

// ========================================
// KUBERNETES LABEL/ANNOTATION VALIDATION - SHARED UTILITY
// 📋 Refactoring: Extract Method pattern | TDD REFACTOR phase
// Authority: 00-core-development-methodology.mdc
// ========================================
//
// TruncateMapValues truncates map values to comply with Kubernetes length limits.
//
// **Kubernetes Length Limits** (from official docs):
//   - **Label values**: max 63 characters
//   - **Annotation values**: max 256KB (262144 bytes)
//
// **Authority**: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
//
// **Why Shared Utility?** (Refactoring Rationale)
//   - ✅ DRY principle: Was duplicated in CRD Creator (31 lines → 15 lines shared)
//   - ✅ Reusable: Other services (SP, RO, WE) need K8s compliance validation
//   - ✅ Centralized: K8s limit changes only need 1 location update
//   - ✅ Testable: K8s compliance logic tested independently
//
// **Use Cases**:
//   - Gateway: Truncate Prometheus alert labels/annotations before CRD creation
//   - SignalProcessing: Truncate enriched metadata
//   - RemediationOrchestrator: Truncate remediation context labels
//   - WorkflowExecution: Truncate workflow metadata
//
// **Parameters**:
//   - input: Map to truncate (typically labels or annotations from external sources)
//   - maxLength: Maximum allowed length per Kubernetes spec
//
// **Returns**:
//   - map[string]string: New map with truncated values (original preserved)
//   - nil if input is nil (graceful handling)
//
// **Example**:
//
//	labels := map[string]string{
//	    "alertname": "HighMemoryUsage",
//	    "description": "Very long description that exceeds 63 characters and needs truncation",
//	}
//	truncated := k8s.TruncateMapValues(labels, k8s.MaxLabelValueLength)
//	// truncated["description"] == "Very long description that exceeds 63 characters and needs tr"
//
// ========================================
func TruncateMapValues(input map[string]string, maxLength int) map[string]string {
	// Graceful handling of nil input
	if input == nil {
		return nil
	}

	// Create new map to avoid mutating input
	result := make(map[string]string, len(input))

	// Truncate each value that exceeds max length
	for key, value := range input {
		if len(value) > maxLength {
			result[key] = value[:maxLength]
		} else {
			result[key] = value
		}
	}

	return result
}

// ========================================
// KUBERNETES LIMITS - CONSTANTS
// ========================================
//
// These constants define Kubernetes label and annotation value length limits
// as specified in the official Kubernetes documentation.
//
// **Authority**: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
//
// **Label Limits**:
//   - Name: max 63 characters
//   - Value: max 63 characters
//   - Character set: [a-z0-9A-Z.-_]
//
// **Annotation Limits**:
//   - Key: max 253 characters (if prefix present) or 63 characters (if no prefix)
//   - Value: max 256KB (262144 bytes)
//   - Character set: any valid UTF-8
//
// **Note**: We use 262000 bytes for annotations (slightly less than 256KB)
// to leave room for JSON marshaling overhead and metadata.
// ========================================

const (
	// MaxLabelValueLength is the maximum length for Kubernetes label values.
	// Per K8s spec: Label values must be 63 characters or less.
	// Authority: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
	MaxLabelValueLength = 63

	// MaxAnnotationValueLength is the practical maximum for Kubernetes annotation values.
	// Per K8s spec: Annotation values can be up to 256KB (262144 bytes).
	// We use 262000 bytes to leave room for JSON marshaling overhead.
	// Authority: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/#syntax-and-character-set
	MaxAnnotationValueLength = 262000
)
