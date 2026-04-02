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

package executor

import "github.com/go-logr/logr"

// FilterDeclaredParameters strips undeclared parameters from the given map.
// #243: Defense-in-depth — even if KA-side validation is bypassed,
// undeclared parameters (including hallucinated credentials) are blocked.
//
// Semantics:
//   - declared == nil  → no schema available, pass all params through (backward compat)
//   - declared != nil  → keep only params whose key is in the declared set
//
// Stripped parameter names are logged at Info level (values are omitted to avoid
// leaking secrets).
func FilterDeclaredParameters(params map[string]string, declared map[string]bool, logger logr.Logger) map[string]string {
	if declared == nil {
		return params
	}
	filtered := make(map[string]string, len(declared))
	for k, v := range params {
		if declared[k] {
			filtered[k] = v
		} else {
			logger.Info("Stripped undeclared parameter", "parameter", k)
		}
	}
	return filtered
}
