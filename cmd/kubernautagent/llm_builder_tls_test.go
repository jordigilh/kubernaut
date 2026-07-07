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

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
)

var _ = Describe("buildTransportChain — TLS wiring", func() {
	It("returns a non-nil transport when tlsCaFile is set", func() {
		caPath := generateTestCACert(GinkgoTB(), "Test CA")

		cfg := kaconfig.DefaultConfig()
		cfg.AI.LLM.TLSCaFile = caPath

		merged := mergeLLMConfig(cfg.AI.LLM, &kaconfig.LLMRuntimeConfig{})

		transport, err := buildTransportChain(merged)
		Expect(err).NotTo(HaveOccurred())
		Expect(transport).NotTo(BeNil())
	})

	It("returns a nil transport when no custom TLS config is set", func() {
		cfg := kaconfig.DefaultConfig()

		merged := mergeLLMConfig(cfg.AI.LLM, &kaconfig.LLMRuntimeConfig{})

		transport, err := buildTransportChain(merged)
		Expect(err).NotTo(HaveOccurred())
		Expect(transport).To(BeNil())
	})

	// UT-KA-1342-030: buildTransportChain returns error for invalid CA file (fail-hard per SC-8)
	It("returns an error for an invalid CA file (fail-hard per SC-8)", func() {
		cfg := kaconfig.DefaultConfig()
		cfg.AI.LLM.TLSCaFile = "/nonexistent/ca.crt"

		merged := mergeLLMConfig(cfg.AI.LLM, &kaconfig.LLMRuntimeConfig{})

		_, err := buildTransportChain(merged)
		Expect(err).To(HaveOccurred())
	})

	// UT-KA-1342-020: buildTransportChain passes WithClientCert when cert fields are set
	It("builds a non-nil transport for a full mTLS config (ca + cert + key)", func() {
		caPath := generateTestCACert(GinkgoTB(), "Test CA")
		certPath, keyPath := generateTestClientCert(GinkgoTB(), caPath)

		cfg := kaconfig.DefaultConfig()
		cfg.AI.LLM.TLSCaFile = caPath
		cfg.AI.LLM.TLSCertFile = certPath
		cfg.AI.LLM.TLSKeyFile = keyPath

		merged := mergeLLMConfig(cfg.AI.LLM, &kaconfig.LLMRuntimeConfig{})

		chain, err := buildTransportChain(merged)
		Expect(err).NotTo(HaveOccurred())
		Expect(chain).NotTo(BeNil())
	})
})
