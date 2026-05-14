/*
Copyright 2026 Jordi Gil.

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

package mcp

// Pattern B (delegated impersonation via apifrontend) uses JWT-based identity
// delegation. The apifrontend forwards the user's original Keycloak JWT, and
// KA's CompositeAuthenticator validates the signature via JWKS and extracts
// identity from verified claims.
//
// The previous SA-token + Impersonate-* header approach was superseded because
// the middleware unconditionally strips Impersonate-* headers (#896), and
// unsigned headers lack the cryptographic integrity of JWT claims.
//
// Implementation: pkg/shared/auth/jwt_auth.go, pkg/shared/auth/composite_auth.go
// Reference: DD-AUTH-MCP-001 v2.0, #895, #896, #1009.
