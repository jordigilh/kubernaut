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

package main

import (
	"testing"

	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
)

// IT-AA-2575-001: the production AIAnalysis client scheme must recognize the
// RemediationWorkflow objects read by the trusted catalog verifier.
func TestAIAnalysisSchemeIncludesRemediationWorkflow(t *testing.T) {
	for _, kind := range []string{"RemediationWorkflow", "RemediationWorkflowList"} {
		if gvk := remediationworkflowv1.GroupVersion.WithKind(kind); !scheme.Recognizes(gvk) {
			t.Errorf("BR-AI-007 / #2575: production scheme does not recognize %s", gvk)
		}
	}
}
