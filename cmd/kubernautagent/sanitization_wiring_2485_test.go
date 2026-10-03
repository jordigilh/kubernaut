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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
)

var _ = Describe("KA sanitization wiring — #2485 (BR-KA-211)", func() {
	It("IT-KA-2485-003 (BR-KA-211 FR-1/FR-2): wires context-aware Secret redaction before G4 and I1", func() {
		cfg := kaconfig.DefaultConfig()

		pipeline := buildSanitizationPipeline(cfg, logr.Discard())

		Expect(pipeline).NotTo(BeNil())
		Expect(pipeline.StageNames()).To(Equal([]string{"K8S-SECRET", "G4", "I1"}))
	})
})
