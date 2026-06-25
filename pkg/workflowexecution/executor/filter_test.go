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

package executor_test

import (
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

// UT-WE-054-FILTER: FilterDeclaredParameters unit tests
// Authority: BR-WE-014 (#243 defense-in-depth parameter filtering)
// FedRAMP: SI-10 (Information Input Validation) -- strip undeclared params
var _ = Describe("UT-WE-054-FILTER: FilterDeclaredParameters", func() {
	var logger logr.Logger

	BeforeEach(func() {
		logger = logr.Discard()
	})

	It("UT-WE-054-FILTER-001: should pass all params when declared is nil (no schema)", func() {
		params := map[string]string{"A": "1", "B": "2"}
		result := executor.FilterDeclaredParameters(params, nil, logger)
		Expect(result).To(Equal(params))
	})

	It("UT-WE-054-FILTER-002: should strip undeclared params when schema exists", func() {
		params := map[string]string{"TIMEOUT": "30s", "SECRET_KEY": "leaked", "MODE": "fast"}
		declared := map[string]bool{"TIMEOUT": true, "MODE": true}

		result := executor.FilterDeclaredParameters(params, declared, logger)
		Expect(result).To(HaveLen(2))
		Expect(result).To(HaveKeyWithValue("TIMEOUT", "30s"))
		Expect(result).To(HaveKeyWithValue("MODE", "fast"))
		Expect(result).ToNot(HaveKey("SECRET_KEY"))
	})

	It("UT-WE-054-FILTER-003: should strip all when declared is empty", func() {
		params := map[string]string{"A": "1", "B": "2"}
		declared := map[string]bool{}

		result := executor.FilterDeclaredParameters(params, declared, logger)
		Expect(result).To(BeEmpty())
	})
})
