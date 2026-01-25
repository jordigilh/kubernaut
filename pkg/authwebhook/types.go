package authwebhook

import (
	"fmt"

	authenticationv1 "k8s.io/api/authentication/v1"
)

// AuthContext holds authenticated user information extracted from admission requests.
// This struct is used for SOC2 CC8.1 operator attribution and audit trail persistence.
// Test Plan Reference: AUTH-001 to AUTH-012
type AuthContext struct {
	Username string
	UID      string
	Groups   []string
	Extra    map[string]authenticationv1.ExtraValue
}

// String returns formatted authentication string for audit trails
// Format: "username (UID: uid)"
func (a *AuthContext) String() string {
	return fmt.Sprintf("%s (UID: %s)", a.Username, a.UID)
}
