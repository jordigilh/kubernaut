// Package hash provides a canonical hashing utility for Kubernetes resource specs.
//
// DD-EM-002: Both the Remediation Orchestrator and Effectiveness Monitor use
// CanonicalResourceFingerprint to compute deterministic, order-independent SHA-256 hashes
// of resource functional state. This enables cross-process pre/post remediation
// comparison without being affected by Go's non-deterministic map iteration
// or Kubernetes API server slice reordering.
//
// Guarantees:
//   - Idempotent: same logical content always produces the same hash
//   - Map-order independent: key iteration order does not affect output
//   - Slice-order independent: element reordering does not affect output
//   - Cross-process portable: separate binaries produce identical hashes
//   - Format: "sha256:<64-lowercase-hex>" (71 characters total)
package hash

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
)

// strippedKeys are the top-level keys excluded from the resource fingerprint.
// These are non-functional metadata managed by Kubernetes, not the resource's
// intended state.
var strippedKeys = map[string]bool{
	"apiVersion": true,
	"kind":       true,
	"metadata":   true,
	"status":     true,
}

// CanonicalResourceFingerprint computes a deterministic SHA-256 hash of a
// Kubernetes resource's functional state (DD-EM-002 v2.0, #765).
//
// It strips non-functional top-level keys (apiVersion, kind, metadata, status)
// and hashes the remaining map. For a Deployment this is {spec: {...}}, for a
// ConfigMap it's {data: {...}, binaryData: {...}}, for a ClusterRole it's
// {rules: [...], aggregationRule: {...}}.
//
// Normalization guarantees:
//   - Map-order independent
//   - Slice-order independent
//   - Idempotent
//   - Format: "sha256:<64-lowercase-hex>" (71 characters)
func CanonicalResourceFingerprint(obj map[string]interface{}) (string, error) {
	if obj == nil {
		obj = map[string]interface{}{}
	}

	functional := make(map[string]interface{}, len(obj))
	for k, v := range obj {
		if strippedKeys[k] {
			continue
		}
		functional[k] = v
	}

	normalized := normalizeValue(functional)

	data, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("failed to marshal normalized resource fingerprint: %w", err)
	}

	h := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", h), nil
}

// normalizeValue recursively normalizes a value for canonical serialization.
// Maps are returned with recursively normalized values. Slices are sorted by
// the canonical JSON of each element after normalization.
func normalizeValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(val))
		for k, child := range val {
			result[k] = normalizeValue(child)
		}
		return result
	case []interface{}:
		normalized := make([]interface{}, len(val))
		for i, child := range val {
			normalized[i] = normalizeValue(child)
		}
		sort.Slice(normalized, func(i, j int) bool {
			ji, _ := json.Marshal(normalized[i])
			jj, _ := json.Marshal(normalized[j])
			return string(ji) < string(jj)
		})
		return normalized
	default:
		return v
	}
}
