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

package tools

import mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"

// ResolveUser returns a UserInfo built from the tool input's acting_user fields
// when present (trusted intermediary model, #1287). When acting_user is empty,
// it falls back to the identity extracted by auth middleware (Pattern A).
func ResolveUser(middlewareUser mcpinternal.UserInfo, actingUser string, actingUserGroups []string) mcpinternal.UserInfo {
	if actingUser != "" {
		return mcpinternal.UserInfo{
			Username: actingUser,
			Groups:   actingUserGroups,
		}
	}
	return middlewareUser
}
