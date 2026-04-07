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

package conversation

import "fmt"

// WorkflowCatalog provides workflow validation.
type WorkflowCatalog interface {
	Exists(workflowID string) bool
}

// OverrideAdvisor validates workflow overrides during conversations.
type OverrideAdvisor struct {
	catalog WorkflowCatalog
}

// NewOverrideAdvisor creates an advisor backed by the workflow catalog.
func NewOverrideAdvisor(catalog WorkflowCatalog) *OverrideAdvisor {
	return &OverrideAdvisor{catalog: catalog}
}

// ValidateOverride checks if the requested workflow exists in the catalog
// and if the session state permits overrides.
func (o *OverrideAdvisor) ValidateOverride(workflowID string, session *Session) (bool, string) {
	if !session.IsInteractive() {
		return false, fmt.Sprintf("overrides are not permitted in %s sessions (read-only or closed)", session.State)
	}
	if !o.catalog.Exists(workflowID) {
		return false, fmt.Sprintf("workflow %q not found in catalog", workflowID)
	}
	return true, ""
}
