package authwebhook

import "fmt"

// AuthContext holds authenticated user information extracted from admission requests
type AuthContext struct {
	Username string
	UID      string
}

// String returns formatted authentication string for audit trails
// Format: "username (UID: uid)"
func (a *AuthContext) String() string {
	return fmt.Sprintf("%s (UID: %s)", a.Username, a.UID)
}
