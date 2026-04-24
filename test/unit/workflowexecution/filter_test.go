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
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

// ========================================
// #243: FilterDeclaredParameters Unit Tests
// Defense-in-depth parameter filtering
// ========================================

var _ = Describe("FilterDeclaredParameters (#243)", func() {
	var logger logr.Logger

	BeforeEach(func() {
		logger = logr.Discard()
	})

	It("UT-WE-243-001: should pass through all params when all are declared", func() {
		params := map[string]string{
			"NAMESPACE": "default",
			"REPLICAS":  "3",
		}
		declared := map[string]bool{
			"NAMESPACE": true,
			"REPLICAS":  true,
		}

		filtered := executor.FilterDeclaredParameters(params, declared, logger)
		Expect(filtered).To(HaveLen(2))
		Expect(filtered).To(HaveKeyWithValue("NAMESPACE", "default"))
		Expect(filtered).To(HaveKeyWithValue("REPLICAS", "3"))
	})

	It("UT-WE-243-002: should strip undeclared params and keep declared ones", func() {
		params := map[string]string{
			"NAMESPACE":    "default",
			"REPLICAS":     "3",
			"HALLUCINATED": "should-not-pass",
			"INJECTED_KEY": "malicious-value",
		}
		declared := map[string]bool{
			"NAMESPACE": true,
			"REPLICAS":  true,
		}

		filtered := executor.FilterDeclaredParameters(params, declared, logger)
		Expect(filtered).To(HaveLen(2))
		Expect(filtered).To(HaveKeyWithValue("NAMESPACE", "default"))
		Expect(filtered).To(HaveKeyWithValue("REPLICAS", "3"))
		Expect(filtered).NotTo(HaveKey("HALLUCINATED"))
		Expect(filtered).NotTo(HaveKey("INJECTED_KEY"))
	})

	It("UT-WE-243-003: should pass through all params when declared is nil (no schema, backward compat)", func() {
		params := map[string]string{
			"NAMESPACE": "default",
			"REPLICAS":  "3",
		}

		filtered := executor.FilterDeclaredParameters(params, nil, logger)
		Expect(filtered).To(HaveLen(2))
		Expect(filtered).To(HaveKeyWithValue("NAMESPACE", "default"))
		Expect(filtered).To(HaveKeyWithValue("REPLICAS", "3"))
	})

	It("UT-WE-243-004: should strip all params when declared is non-nil empty map", func() {
		params := map[string]string{
			"NAMESPACE": "default",
			"REPLICAS":  "3",
		}
		declared := map[string]bool{}

		filtered := executor.FilterDeclaredParameters(params, declared, logger)
		Expect(filtered).To(BeEmpty())
	})

	It("UT-WE-243-005: should return empty map when params is empty", func() {
		params := map[string]string{}
		declared := map[string]bool{
			"NAMESPACE": true,
		}

		filtered := executor.FilterDeclaredParameters(params, declared, logger)
		Expect(filtered).To(BeEmpty())
	})

	It("UT-WE-243-006: should return empty map when both params and declared are empty", func() {
		filtered := executor.FilterDeclaredParameters(map[string]string{}, map[string]bool{}, logger)
		Expect(filtered).To(BeEmpty())
	})

	It("UT-WE-243-007: should return nil params unchanged when declared is nil", func() {
		filtered := executor.FilterDeclaredParameters(nil, nil, logger)
		Expect(filtered).To(BeNil())
	})
})
