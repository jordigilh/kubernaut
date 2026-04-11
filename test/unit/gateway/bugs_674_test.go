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

package gateway

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gwconfig "github.com/jordigilh/kubernaut/pkg/gateway/config"
)

// Issue #674 Bug 4: Gateway LoadFromFile swallows all errors silently.
// TDD RED phase — these tests MUST FAIL before the fix is applied.
var _ = Describe("Issue #674 Bug 4: Gateway LoadFromFile error propagation (BR-PLATFORM-003)", func() {

	It("UT-GW-674-001: nonexistent config file returns error", func() {
		_, err := gwconfig.LoadFromFile("/nonexistent/path/config.yaml")
		Expect(err).To(HaveOccurred(), "nonexistent file should return error, not silent defaults")
	})

	It("UT-GW-674-002: malformed YAML returns error", func() {
		tmpFile, err := os.CreateTemp("", "gw-bad-config-*.yaml")
		Expect(err).ToNot(HaveOccurred())
		defer os.Remove(tmpFile.Name())

		_, err = tmpFile.WriteString("{{{{invalid yaml content")
		Expect(err).ToNot(HaveOccurred())
		tmpFile.Close()

		_, err = gwconfig.LoadFromFile(tmpFile.Name())
		Expect(err).To(HaveOccurred(), "malformed YAML should return error, not silent defaults")
	})

	It("UT-GW-674-003: valid YAML returns parsed config without error", func() {
		tmpFile, err := os.CreateTemp("", "gw-good-config-*.yaml")
		Expect(err).ToNot(HaveOccurred())
		defer os.Remove(tmpFile.Name())

		_, err = tmpFile.WriteString("server:\n  listenAddr: \":9090\"\n")
		Expect(err).ToNot(HaveOccurred())
		tmpFile.Close()

		cfg, err := gwconfig.LoadFromFile(tmpFile.Name())
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg.Server.ListenAddr).To(Equal(":9090"), "YAML listenAddr should be parsed into config")
	})
})
