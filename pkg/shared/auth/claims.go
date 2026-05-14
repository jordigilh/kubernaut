package auth

import (
	"fmt"
	"strings"
)

// extractClaim resolves a dot-notation path against a JWT claims map.
// Supports nested paths like "realm_access.roles" which traverses
// claims["realm_access"].(map)["roles"].
//
// Returns the resolved value or an error if any intermediate key is
// missing or not a map. The caller is responsible for type-asserting
// the result (string for username, []interface{} for groups).
func ExtractClaim(claims map[string]interface{}, path string) (interface{}, error) {
	parts := strings.Split(path, ".")
	var current interface{} = claims

	for i, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("claim path %q: segment %q is not a map (at index %d)", path, parts[i-1], i-1)
		}
		val, exists := m[part]
		if !exists {
			return nil, fmt.Errorf("claim path %q: key %q not found", path, part)
		}
		current = val
	}

	return current, nil
}

// extractStringClaim resolves a dot-notation path and asserts the result is a string.
func ExtractStringClaim(claims map[string]interface{}, path string) (string, error) {
	val, err := ExtractClaim(claims, path)
	if err != nil {
		return "", err
	}
	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("claim path %q: expected string, got %T", path, val)
	}
	return s, nil
}

// extractGroupsClaim resolves a dot-notation path and asserts the result is a
// slice of strings. Handles both []interface{} (standard JSON unmarshal) and
// []string formats.
func ExtractGroupsClaim(claims map[string]interface{}, path string) ([]string, error) {
	val, err := ExtractClaim(claims, path)
	if err != nil {
		return nil, err
	}

	switch v := val.(type) {
	case []interface{}:
		groups := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("claim path %q: element %d is %T, expected string", path, i, item)
			}
			groups = append(groups, s)
		}
		return groups, nil
	case []string:
		return v, nil
	default:
		return nil, fmt.Errorf("claim path %q: expected array, got %T", path, val)
	}
}
