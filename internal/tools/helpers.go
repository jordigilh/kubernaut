package tools

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

// ErrNotFound indicates the requested resource was not found.
var ErrNotFound = errors.New("not found")

// ErrForbidden indicates the user does not have access.
var ErrForbidden = errors.New("access denied")

// ErrAlreadyTerminal indicates the resource is already in a terminal state.
var ErrAlreadyTerminal = errors.New("already in terminal state")

// ErrK8sUnavailable indicates the K8s cluster is not reachable.
var ErrK8sUnavailable = errors.New("kubernetes cluster is not available — contact your administrator")

// ParseRRID parses an rr_id shorthand (namespace/name) into its components.
// If rr_id is empty, namespace and name are returned as-is.
func ParseRRID(rrID, namespace, name string) (ns, n string, err error) {
	if rrID != "" {
		parts := strings.SplitN(rrID, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", "", fmt.Errorf("invalid rr_id format %q: expected namespace/name", rrID)
		}
		return parts[0], parts[1], nil
	}
	if namespace == "" || name == "" {
		return "", "", fmt.Errorf("namespace and name are required when rr_id is not provided")
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
			return fmt.Errorf("operation failed (code %d) — contact your cluster administrator", statusErr.ErrStatus.Code)
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
