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

package investigator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

var _ = Describe("Kubernaut Agent Registry Wiring — TP-433-WIR Phase 2", func() {

	Describe("IT-KA-433W-007: Registry with DS URL includes all 5 custom tools", func() {
		It("should register all 5 custom tool names when DataStorage is configured", func() {
			reg := registry.New()
			custom.RegisterAll(reg, nil, nil, nil)

			allTools := reg.All()
			toolNames := make([]string, len(allTools))
			for i, t := range allTools {
				toolNames[i] = t.Name()
			}
			Expect(toolNames).To(ConsistOf(custom.AllToolNames))
		})
	})

	Describe("IT-KA-433W-008: Registry without DS URL excludes custom tools", func() {
		It("should have zero custom tools when DataStorage is not configured", func() {
			reg := registry.New()

			allTools := reg.All()
			toolNames := make([]string, len(allTools))
			for i, t := range allTools {
				toolNames[i] = t.Name()
			}
			for _, customName := range custom.AllToolNames {
				Expect(toolNames).NotTo(ContainElement(customName))
			}
		})
	})
})
