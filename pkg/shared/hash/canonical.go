// Package hash provides a canonical hashing utility for Kubernetes resource specs.
//
// DD-EM-002: Both the Remediation Orchestrator and Effectiveness Monitor use
// CanonicalSpecHash to compute deterministic, order-independent SHA-256 hashes
// of resource .spec fields. This enables cross-process pre/post remediation
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

// CanonicalSpecHash computes a deterministic SHA-256 hash of a Kubernetes
// resource spec (DD-EM-002). The algorithm recursively normalizes the input:
//
//   - Maps: keys sorted alphabetically, values normalized recursively
//   - Slices: elements sorted by their canonical JSON representation
//   - Scalars: passed through unchanged
//
// A nil spec is treated as an empty map. The returned string is in the format
// "sha256:<64-lowercase-hex>" (71 characters total).
func CanonicalSpecHash(spec map[string]interface{}) (string, error) {
	if spec == nil {
		spec = map[string]interface{}{}
	}

	normalized := normalizeValue(spec)

	data, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("failed to marshal normalized spec: %w", err)
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
