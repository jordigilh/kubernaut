// Package validate provides reusable input validation helpers for Kubernetes
// resource names and namespaces, wrapping k8s.io/apimachinery/pkg/util/validation
// to present user-friendly error messages.
package validate

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

var kindRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

// Namespace validates that ns is a valid Kubernetes namespace (RFC 1123 DNS label).
func Namespace(ns string) error {
	if ns == "" {
		return fmt.Errorf("namespace must not be empty")
	}
	if errs := validation.IsDNS1123Label(ns); len(errs) > 0 {
		return fmt.Errorf("invalid namespace %q: %s", ns, strings.Join(errs, "; "))
	}
	return nil
}

// ResourceName validates that name is a valid Kubernetes resource name (RFC 1123 DNS subdomain).
func ResourceName(name string) error {
	if name == "" {
		return fmt.Errorf("resource name must not be empty")
	}
	if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return fmt.Errorf("invalid resource name %q: %s", name, strings.Join(errs, "; "))
	}
	return nil
}

// Kind validates that k is a valid Kubernetes resource kind (PascalCase identifier, ASCII alphanumeric only).
func Kind(k string) error {
	if k == "" {
		return fmt.Errorf("kind must not be empty")
	}
	if len(k) > 63 {
		return fmt.Errorf("kind %q exceeds max length 63", k)
	}
	if !kindRE.MatchString(k) {
		return fmt.Errorf("invalid kind %q: must start with letter and contain only ASCII alphanumeric characters", k)
	}
	return nil
}

// ParseRRID validates and splits an rr_id in the form "namespace/name" into
// its components. Returns an error for empty, malformed, or path-traversal values.
func ParseRRID(rrid string) (namespace, name string, err error) {
	if rrid == "" {
		return "", "", fmt.Errorf("rr_id must not be empty")
	}
	if strings.Contains(rrid, "..") {
		return "", "", fmt.Errorf("rr_id %q contains path traversal", rrid)
	}
	parts := strings.SplitN(rrid, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("rr_id %q must be in the form namespace/name", rrid)
	}
	if err := Namespace(parts[0]); err != nil {
		return "", "", fmt.Errorf("rr_id namespace: %w", err)
	}
	if err := ResourceName(parts[1]); err != nil {
		return "", "", fmt.Errorf("rr_id name: %w", err)
	}
	return parts[0], parts[1], nil
}

// ValidActions is the set of valid interactive action strings.
var ValidActions = map[string]bool{
	"investigate":  true,
	"discover":     true,
	"select":       true,
	"takeover":     true,
	"message":      true,
	"complete":     true,
	"cancel":       true,
	"status":       true,
	"reconnect":    true,
}

// Action validates that the action string is in the valid set.
func Action(action string) error {
	if !ValidActions[action] {
		return fmt.Errorf("invalid action %q: must be one of investigate, discover, select, takeover, message, complete, cancel, status, reconnect", action)
	}
	return nil
}

// MaxMessageLen is the maximum allowed length for interactive message content.
const MaxMessageLen = 10240

// MessageLength validates that msg is within the allowed length.
func MessageLength(msg string) error {
	if len(msg) > MaxMessageLen {
		return fmt.Errorf("message length %d exceeds maximum %d", len(msg), MaxMessageLen)
	}
	return nil
}

// LabelValue validates that v is a valid Kubernetes label value.
func LabelValue(v string) error {
	if errs := validation.IsValidLabelValue(v); len(errs) > 0 {
		return fmt.Errorf("invalid label value %q: %s", v, strings.Join(errs, "; "))
	}
	return nil
}
