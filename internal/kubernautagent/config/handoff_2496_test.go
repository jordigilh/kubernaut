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

package config_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
)

// DD-KA-2496 / BR-KA-016: defaults are independent of model-context truncation.
var _ = Describe("#2496 positive independent evidence handoff budget", func() {
	It("UT-KA-2496-010: defaults to 65,536 bytes when omitted", func() {
		cfg, err := config.Load([]byte("ai:\n  summarizer:\n    maxToolOutputSize: 32\n"))
		Expect(err).NotTo(HaveOccurred())
		raw, err := yaml.Marshal(cfg.AI.Investigation)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).To(ContainSubstring("maxEvidenceHandoffBytes: 65536"))
	})
	DescribeTable("UT-KA-2496-010: rejects non-positive explicitly configured budgets",
		func(budget int) {
			cfg, err := config.Load([]byte(fmt.Sprintf("ai:\n  investigation:\n    maxEvidenceHandoffBytes: %d\n", budget)))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("maxEvidenceHandoffBytes must be positive")))
		}, Entry("zero", 0), Entry("negative", -1),
	)
})
