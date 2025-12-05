/*
Copyright 2025 Jordi Gil.

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

package remediationorchestrator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ro "github.com/jordigilh/kubernaut/pkg/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

var _ = Describe("Interfaces", func() {

	Describe("PhaseManager interface", func() {

		It("should be satisfied by phase.Manager", func() {
			// Given: A phase.Manager instance
			manager := phase.NewManager()

			// When: We assign it to the interface type
			var iface ro.PhaseManager = manager

			// Then: The interface should be satisfied (compile-time check)
			Expect(iface).ToNot(BeNil())
		})
	})
})
