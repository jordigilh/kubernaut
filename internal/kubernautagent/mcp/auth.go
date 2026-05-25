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

// AF-to-KA Authentication: Trusted Intermediary Model (#1287)
//
// AF authenticates to KA using its ServiceAccount token (same pattern as
// AF-to-DS). KA validates the SA token via Kubernetes TokenReview (Pattern A).
// User identity flows through the MCP tool payload as acting_user and
// acting_user_groups fields, used for session ownership and audit attribution.
//
// This supersedes the JWT delegation model (DD-AUTH-MCP-001 v2.0, ADR-013)
// where AF forwarded the user's Keycloak JWT for KA to validate. The trusted
// intermediary model simplifies KA by removing external IDP integration:
//   - AF handles user AuthN/AuthZ (OIDC, RBAC)
//   - KA handles service AuthZ only (AF SA must have permission to call KA)
//   - User identity in payload is audit-only, not used for KA authorization
//
// Identity resolution: tools.ResolveUser prefers acting_user from payload
// when present, falling back to middleware-extracted identity for Pattern A.
//
// History: SA+Impersonate (#891) → JWT delegation (#895, #896, #1009) →
// Trusted intermediary (#1287).
// Reference: #1287, #1288.
