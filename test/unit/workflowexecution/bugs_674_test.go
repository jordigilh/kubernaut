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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	weconfig "github.com/jordigilh/kubernaut/pkg/workflowexecution/config"
)

// Issue #674 Bug 4: WorkflowExecution LoadFromFile swallows all errors silently.
// TDD RED phase — this test MUST FAIL before the fix is applied.
var _ = Describe("Issue #674 Bug 4: WorkflowExecution LoadFromFile error propagation (BR-PLATFORM-003)", func() {

	It("UT-WE-674-001: nonexistent config file returns error", func() {
		_, err := weconfig.LoadFromFile("/nonexistent/path/config.yaml")
		Expect(err).To(HaveOccurred(), "nonexistent file should return error, not silent defaults")
	})
})
