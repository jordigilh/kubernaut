package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

// AuditableInput is an opt-in interface that tool argument types can implement
// to enrich audit events with resource-specific context (e.g., namespace, resource name).
// Fields returned by AuditFields are merged into the audit event detail map on success.
type AuditableInput interface {
	AuditFields() map[string]string
}

// ErrNotFound indicates the requested resource was not found.
var ErrNotFound = errors.New("not found")

// ErrForbidden indicates the user does not have access.
var ErrForbidden = errors.New("access denied")

// ErrAlreadyTerminal indicates the resource is already in a terminal state.
var ErrAlreadyTerminal = errors.New("already in terminal state")

// ErrK8sUnavailable indicates the K8s cluster is not reachable.
var ErrK8sUnavailable = errors.New("kubernetes cluster is not available — contact your administrator")

// ErrInvalidInput indicates input validation failed (RFC 1123, empty fields, etc.).
var ErrInvalidInput = errors.New("invalid input")

// maxToolOutputBytes is the maximum serialized output size for tool results.
// Matches the 4KB threshold used by session.TrimToolResult for etcd safety.
const maxToolOutputBytes = 4096

// ParseRRID resolves rr_id to namespace and name. The rr_id is always a plain
// resource name (no namespace prefix). If rr_id is empty, the explicit
// namespace and name arguments are used as fallback.
func ParseRRID(rrID, namespace, name string) (ns, n string, err error) {
	if rrID != "" {
		return namespace, rrID, nil
	}
	if name == "" {
		return "", "", fmt.Errorf("name is required when rr_id is not provided")
	}
	return namespace, name, nil
}

// ParseResourceID parses a resource ID shorthand (namespace/name) into its components.
// If resourceID is empty, namespace and name are returned as-is.
func ParseResourceID(resourceID, namespace, name string) (ns, n string, err error) {
	if resourceID != "" {
		parts := strings.SplitN(resourceID, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", "", fmt.Errorf("invalid resource_id format %q: expected namespace/name", resourceID)
		}
		return parts[0], parts[1], nil
	}
	if namespace == "" || name == "" {
		return "", "", fmt.Errorf("namespace and name are required when resource_id is not provided")
	}
	return namespace, name, nil
}

// ToUserFriendlyError translates K8s API errors into user-friendly messages.
// Internal details (namespace paths, resource versions, field paths) are not exposed.
func ToUserFriendlyError(err error) error {
	if err == nil {
		return nil
	}

	var statusErr *k8serrors.StatusError
	if errors.As(err, &statusErr) {
		switch statusErr.ErrStatus.Code {
		case http.StatusForbidden:
			return fmt.Errorf("%w: %s", ErrForbidden, buildForbiddenMsg(statusErr.ErrStatus.Message))
		case http.StatusNotFound:
			return fmt.Errorf("%w: the requested resource does not exist", ErrNotFound)
		case http.StatusConflict:
			return fmt.Errorf("operation conflict — the resource was modified concurrently, please retry")
		default:
			return fmt.Errorf("operation failed (code %d): the server could not process the request", statusErr.ErrStatus.Code)
		}
	}
	return err
}

func buildForbiddenMsg(msg string) string {
	parts := strings.SplitN(msg, "cannot", 2)
	if len(parts) == 2 {
		action := strings.TrimSpace(parts[1])
		if idx := strings.Index(action, "in API group"); idx > 0 {
			action = strings.TrimSpace(action[:idx])
		}
		return fmt.Sprintf("you lack access to %s -- contact your cluster administrator for RBAC permissions", action)
	}
	return "you lack access to this resource -- contact your cluster administrator for RBAC permissions"
}

// IsTerminalPhase returns true if the given RR phase is terminal.
func IsTerminalPhase(phase string) bool {
	switch phase {
	case "Completed", "Failed", "Cancelled":
		return true
	}
	return false
}

// TrimSliceToFit removes trailing elements from a slice until its JSON
// serialization fits within maxToolOutputBytes, returning the trimmed slice
// and whether any trimming occurred. The marshal function should serialize
// the slice to JSON bytes.
func TrimSliceToFit[T any](items []T) ([]T, bool) {
	output, err := json.Marshal(items)
	if err != nil {
		return items, false
	}
	if len(output) <= maxToolOutputBytes {
		return items, false
	}
	for len(items) > 1 {
		items = items[:len(items)-1]
		output, err = json.Marshal(items)
		if err != nil || len(output) <= maxToolOutputBytes {
			break
		}
	}
	return items, true
}
