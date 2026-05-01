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

// Pattern B (delegated impersonation via apifrontend) is deferred until that
// component exists. The previous ExtractEffectiveUser implementation was
// architecturally incompatible with the shared auth middleware which strips
// Impersonate-* headers before any handler sees them (#896).
//
// When apifrontend ships, Pattern B should use a trust-boundary mechanism
// (signed JWT, mTLS client cert, or internal-only header set by the gateway)
// rather than raw Impersonate-* headers from external clients.
//
// Reference: DD-AUTH-MCP-001, #895, #896.
