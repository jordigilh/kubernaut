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

package authwebhook

import (
	"fmt"

	authenticationv1 "k8s.io/api/authentication/v1"
)

// AuthContext contains the authenticated user information from Kubernetes
// Extracted from admission.Request.UserInfo by Kubernetes API Server
// Used for SOC2 CC8.1 (Attribution) compliance
type AuthContext struct {
	// Username: Authenticated username from K8s auth (OIDC, cert, SA)
	Username string

	// UID: Unique identifier for the user
	UID string

	// Groups: Groups the user belongs to
	Groups []string

	// Extra: Additional attributes from authentication provider
	Extra map[string]authenticationv1.ExtraValue
}

// String returns a formatted authentication string for audit trail
// Format: "username (UID: uid)"
// Used in audit events and CRD status fields
func (a *AuthContext) String() string {
	return fmt.Sprintf("%s (UID: %s)", a.Username, a.UID)
}

