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

package types

import (
	"crypto/sha256"
	"fmt"
)

// ========================================
// FINGERPRINT GENERATION - SHARED UTILITY
// 📋 Refactoring: Extract Method pattern | TDD REFACTOR phase
// Authority: 00-core-development-methodology.mdc
// ========================================
//
// CalculateFingerprint generates a unique fingerprint for signal deduplication.
//
// **Business Requirement**: BR-GATEWAY-069 (Deduplication tracking)
//
// **Fingerprint Algorithm**:
//   - Format: SHA256(identifier:namespace:kind:name)
//   - Deterministic: Same signal inputs → same fingerprint
//   - Collision-resistant: SHA256 provides 256-bit uniqueness
//
// **Examples**:
//   - Prometheus Alert: SHA256("HighMemoryUsage:prod:Pod:payment-api-789")
//   - K8s Event:        SHA256("OOMKilled:prod:Pod:payment-api-789")
//
// **Shared Across Adapters**:
//   - PrometheusAdapter: Uses alertName as identifier
//   - KubernetesEventAdapter: Uses event.Reason as identifier
//   - Future adapters: Consistent deduplication behavior guaranteed
//
// **Why Shared Utility?** (Refactoring Rationale)
//   - ✅ Single source of truth: Algorithm changes affect ALL adapters consistently
//   - ✅ Eliminates duplication: Was duplicated in 2 adapter files
//   - ✅ Future-proof: New adapters automatically use correct algorithm
//   - ✅ Testable: Core business logic tested independently
//
// **Parameters**:
//   - identifier: Alert name (Prometheus) or Event reason (K8s) - business identifier
//   - resource: Target resource for remediation (namespace, kind, name)
//
// **Returns**:
//   - string: Hex-encoded SHA256 hash (64 characters)
//
// ========================================
func CalculateFingerprint(identifier string, resource ResourceIdentifier) string {
	// Build fingerprint input string
	// Format: identifier:namespace:kind:name
	// Example: "HighMemoryUsage:prod-payment-service:Pod:payment-api-789"
	input := fmt.Sprintf("%s:%s:%s:%s",
		identifier,
		resource.Namespace,
		resource.Kind,
		resource.Name,
	)

	// Generate SHA256 hash
	hash := sha256.Sum256([]byte(input))

	// Return hex-encoded hash (64 characters)
	// Example: "bd773c9f25ac1e4d6f8a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e"
	return fmt.Sprintf("%x", hash)
}

// CalculateOwnerFingerprint generates a fingerprint based on the owner resource,
// without including the signal reason/identifier.
//
// **Business Requirement**: Prevents duplicate remediation workflows for events that
// originate from the same root cause (same Deployment/StatefulSet/DaemonSet).
//
// **Fingerprint Algorithm**:
//   - Format: SHA256(namespace:ownerKind:ownerName)
//   - Deterministic: Same owner → same fingerprint regardless of event reason
//   - Used by KubernetesEventAdapter when OwnerResolver is configured
//
// **Examples**:
//   - Pod crash event: SHA256("prod:Deployment:payment-api")
//   - OOM event from same deployment: SHA256("prod:Deployment:payment-api") ← same!
//
// **Parameters**:
//   - resource: The owner resource (namespace, kind, name)
//
// **Returns**:
//   - string: Hex-encoded SHA256 hash (64 characters)
func CalculateOwnerFingerprint(resource ResourceIdentifier) string {
	// Build fingerprint input string WITHOUT reason/identifier
	// Format: namespace:kind:name
	// Example: "prod:Deployment:payment-api"
	input := fmt.Sprintf("%s:%s:%s",
		resource.Namespace,
		resource.Kind,
		resource.Name,
	)

	// Generate SHA256 hash
	hash := sha256.Sum256([]byte(input))

	return fmt.Sprintf("%x", hash)
}
