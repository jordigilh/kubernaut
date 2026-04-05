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

package effectivenessmonitor

import (
	"os"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func emReconcilerPath() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..",
		"internal", "controller", "effectivenessmonitor", "reconciler.go")
}

var _ = Describe("Issue #639: EM Metrics Query Definitions (BR-EM-003)", func() {

	var reconcilerSource string

	BeforeEach(func() {
		data, err := os.ReadFile(emReconcilerPath())
		Expect(err).NotTo(HaveOccurred(), "EM reconciler source should be readable")
		reconcilerSource = string(data)
	})

	It("UT-EM-639-001: CPU query should use rate() wrapper instead of raw counter", func() {
		Expect(reconcilerSource).To(ContainSubstring(`sum(rate(container_cpu_usage_seconds_total`),
			"CPU query must use rate() on the counter metric for meaningful improvement scores")
		Expect(reconcilerSource).To(ContainSubstring(`[5m])`),
			"CPU rate() window should be 5m to match HTTP metric query windows")
	})

	It("UT-EM-639-002: Memory query should use raw sum() (gauge metric, no rate needed)", func() {
		Expect(reconcilerSource).To(ContainSubstring(`sum(container_memory_working_set_bytes{namespace=`),
			"Memory query should use raw sum() since it is a gauge metric")
		Expect(reconcilerSource).NotTo(ContainSubstring(`rate(container_memory_working_set_bytes`),
			"Memory query must NOT use rate() — working_set_bytes is a gauge, not a counter")
	})

	It("UT-EM-639-003: CPU query should be marked as LowerIsBetter", func() {
		Expect(reconcilerSource).To(MatchRegexp(
			`container_cpu_usage_seconds_total[\s\S]{1,200}LowerIsBetter:\s*true`),
			"CPU metric should be LowerIsBetter (lower CPU rate = improvement after remediation)")
	})
})
