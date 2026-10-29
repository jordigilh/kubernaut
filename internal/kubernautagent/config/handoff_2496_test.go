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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
)

// Issue #2521 / BR-KA-016: the raw-evidence tuning knob is no longer part of
// operator configuration; Investigator owns the fixed structured-context guard.
var _ = Describe("Issue #2521 structured Phase 3 context budget configuration", func() {
	It("UT-KA-2521-008: investigation configuration does not expose the raw evidence budget", func() {
		cfg, err := config.Load([]byte("ai:\n  summarizer:\n    maxToolOutputSize: 32\n"))
		Expect(err).NotTo(HaveOccurred())
		raw, err := yaml.Marshal(cfg.AI.Investigation)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).NotTo(ContainSubstring("maxEvidenceHandoffBytes"))
	})
})
