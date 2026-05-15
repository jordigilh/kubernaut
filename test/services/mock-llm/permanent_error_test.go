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
package mockllm_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/fault"
)

var _ = Describe("Permanent Error Logic", func() {

	Describe("UT-MOCK-054-001: FaultInjector can be configured for permanent server error", func() {
		It("should report active fault when configured", func() {
			fi := fault.NewInjector()
			Expect(fi.IsActive()).To(BeFalse())

			fi.Configure(fault.Config{
				Enabled:    true,
				StatusCode: 500,
				Message:    "permanent error",
			})
			Expect(fi.IsActive()).To(BeTrue())
			Expect(fi.StatusCode()).To(Equal(500))
			Expect(fi.Message()).To(Equal("permanent error"))

			fi.Reset()
			Expect(fi.IsActive()).To(BeFalse())
		})
	})
})
