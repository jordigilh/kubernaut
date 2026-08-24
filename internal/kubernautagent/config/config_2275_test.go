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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
)

// BR-PLATFORM-012, Issue #2275: rename Runtime.Server.DisableProfiling
// (default true) to shared, positive-polarity Runtime.Debug.PprofEnabled
// (default false), mirroring the same secure-by-default posture with no
// polarity flip at the kubernaut-operator CRD boundary.
var _ = Describe("BR-PLATFORM-012: Runtime.Debug.PprofEnabled secure default (Issue #2275)", func() {
	It("UT-KA-2275-001 DefaultConfig disables profiling by default (secure-by-default)", func() {
		cfg := config.DefaultConfig()
		Expect(cfg.Runtime.Debug.PprofEnabled).To(BeFalse())
	})

	It("UT-KA-2275-002 YAML can opt in to profiling by setting runtime.debug.pprofEnabled: true", func() {
		data := []byte("runtime:\n  debug:\n    pprofEnabled: true\n")
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Runtime.Debug.PprofEnabled).To(BeTrue())
	})

	It("UT-KA-2275-003 omitted debug in YAML keeps the secure default", func() {
		data := []byte("runtime:\n  server:\n    port: 8443\n")
		cfg, err := config.Load(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Runtime.Debug.PprofEnabled).To(BeFalse())
	})
})
