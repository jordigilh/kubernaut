// Package validate provides reusable input validation helpers for Kubernetes
// resource names and namespaces, wrapping k8s.io/apimachinery/pkg/util/validation
// to present user-friendly error messages.
package validate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	"k8s.io/apimachinery/pkg/util/validation"
)

var (
	kindRE      = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)
	alertNameRE = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
)

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

// RRID validates a Remediation Request ID, which may be in namespace/name format
// or just a plain resource name. Both parts must be valid Kubernetes identifiers.
func RRID(rrid string) error {
	if rrid == "" {
		return fmt.Errorf("rr_id must not be empty")
	}
	parts := strings.SplitN(rrid, "/", 2)
	if len(parts) == 2 {
		if err := Namespace(parts[0]); err != nil {
			return fmt.Errorf("invalid rr_id namespace: %w", err)
		}
		if err := ResourceName(parts[1]); err != nil {
			return fmt.Errorf("invalid rr_id name: %w", err)
		}
		return nil
	}
	return ResourceName(rrid)
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

// ValidActions is the set of valid interactive action strings.
var ValidActions = map[string]bool{
	"investigate": true,
	"discover":    true,
	"select":      true,
	"takeover":    true,
	"message":     true,
	"complete":    true,
	"cancel":      true,
	"status":      true,
	"reconnect":   true,
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

// MaxAlertNameLen is the maximum length for a Prometheus alert name.
const MaxAlertNameLen = 253

// AlertName validates that name is a valid Prometheus alert name.
// Prometheus alert names follow the pattern [a-zA-Z_:][a-zA-Z0-9_:]*.
func AlertName(name string) error {
	if name == "" {
		return fmt.Errorf("alert_name must not be empty")
	}
	if len(name) > MaxAlertNameLen {
		return fmt.Errorf("alert_name %q exceeds max length %d", name, MaxAlertNameLen)
	}
	if !alertNameRE.MatchString(name) {
		return fmt.Errorf("invalid alert_name %q: must match Prometheus naming convention [a-zA-Z_:][a-zA-Z0-9_:]*", name)
	}
	return nil
}

// MaxAPIVersionLen is the maximum length for a Kubernetes API version string.
const MaxAPIVersionLen = 253

// APIVersion validates that v is a valid Kubernetes API version string.
// Delegates to the canonical scope.ParseAPIVersion (#2592 identity contract)
// so the AF tools accept exactly the forms every other component accepts:
// core "v<N>((alpha|beta)<N>)?" and grouped "<DNS1123-subdomain>/<DNS1035-label>"
// (e.g. "v1", "apps/v1", "example.com/stable"). A regex-only check previously
// rejected grouped non-v<N> versions (CRD versions like "stable") that the
// fingerprint's group extraction accepts, splitting the identity contract.
func APIVersion(v string) error {
	if v == "" {
		return fmt.Errorf("api_version must not be empty")
	}
	if len(v) > MaxAPIVersionLen {
		return fmt.Errorf("api_version %q exceeds max length %d", v, MaxAPIVersionLen)
	}
	if _, _, err := scope.ParseAPIVersion(v); err != nil {
		return fmt.Errorf("invalid api_version %q: must be a core 'v<N>' or '<group>/<version>' (e.g. 'v1', 'apps/v1', 'example.com/stable')", v)
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

// MaxClusterIDLen is the maximum length for a fleet cluster identifier (ADR-065).
const MaxClusterIDLen = 253

// ClusterID validates an optional fleet cluster identifier (ADR-065). Empty is
// valid (single-cluster/local-hub deployments carry no cluster attribution).
// Format is intentionally not further constrained here (ADR-065 documents a
// pre-existing format inconsistency across producers, out of scope for this fix).
func ClusterID(clusterID string) error {
	if len(clusterID) > MaxClusterIDLen {
		return fmt.Errorf("cluster_id %q exceeds max length %d", clusterID, MaxClusterIDLen)
	}
	return nil
}

// MaxEscalationReasonLen is the maximum allowed length for an escalation reason.
const MaxEscalationReasonLen = 1024

// EscalationReason validates that reason is within the allowed length and
// does not contain control characters that could break structured logging.
func EscalationReason(reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("escalation_reason must not be empty or whitespace-only")
	}
	if len(reason) > MaxEscalationReasonLen {
		return fmt.Errorf("escalation_reason length %d exceeds maximum %d", len(reason), MaxEscalationReasonLen)
	}
	for i, r := range reason {
		if r < 0x20 && r != '\n' && r != '\t' {
			return fmt.Errorf("escalation_reason contains control character at position %d", i)
		}
	}
	return nil
}
