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

package workflowexecution

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Issue #1661 Change 11e (DD-WORKFLOW-018): this is the first pure-unit
// (non-envtest) Ginkgo suite for internal/controller/workflowexecution.
// Prior to Phase 49, resolveSchemaMetadata/resolveWorkflowCatalog were only
// exercised indirectly through the full envtest reconciler in
// test/integration/workflowexecution. Phase 50 removed both DS call sites
// entirely (WorkflowExecutionReconciler no longer has a WorkflowQuerier
// field at all) -- Pyramid Invariant: UT proves logic, IT (existing suite)
// proves wiring.
func TestWorkflowExecutionControllerUnit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "WorkflowExecution Controller Unit Suite")
}
