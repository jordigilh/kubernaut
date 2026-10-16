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

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	pathCaCrt = "/nonexistent/ca.crt"
)

var _ = Describe("buildTransportChain — TLS wiring", func() {
	It("returns a non-nil transport when tlsCaFile is set", func() {
		caPath := generateTestCACert(GinkgoTB(), "Test CA")

		merged := types.LLMConfig{Provider: types.LLMProviderVertexAI, Model: "claude-sonnet-4-6", VertexProject: "test-project", VertexLocation: "us-central1", TLSCaFile: caPath}

		transport, err := buildTransportChain(merged)
		Expect(err).NotTo(HaveOccurred())
		Expect(transport).NotTo(BeNil())
	})

	// GAP-14 / Issue #1519: buildTransportChain now always returns a non-nil
	// transport (otelhttp-wrapped) so the outbound LLM span exists even on
	// the vanilla default-config path -- this is the "no custom TLS/OAuth2/
	// headers/circuit-breaker" case, previously asserted as nil.
	It("returns a non-nil (otelhttp-wrapped) transport when no custom TLS config is set", func() {
		merged := types.LLMConfig{Provider: types.LLMProviderVertexAI, Model: "claude-sonnet-4-6", VertexProject: "test-project", VertexLocation: "us-central1"}

		transport, err := buildTransportChain(merged)
		Expect(err).NotTo(HaveOccurred())
		Expect(transport).NotTo(BeNil())
	})

	// UT-KA-1342-030: buildTransportChain returns error for invalid CA file (fail-hard per SC-8)
	It("returns an error for an invalid CA file (fail-hard per SC-8)", func() {
		merged := types.LLMConfig{Provider: types.LLMProviderVertexAI, Model: "claude-sonnet-4-6", VertexProject: "test-project", VertexLocation: "us-central1", TLSCaFile: pathCaCrt}

		_, err := buildTransportChain(merged)
		Expect(err).To(HaveOccurred())
	})

	// UT-KA-1342-020: buildTransportChain passes WithClientCert when cert fields are set
	It("builds a non-nil transport for a full mTLS config (ca + cert + key)", func() {
		caPath := generateTestCACert(GinkgoTB(), "Test CA")
		certPath, keyPath := generateTestClientCert(GinkgoTB(), caPath)

		merged := types.LLMConfig{Provider: types.LLMProviderVertexAI, Model: "claude-sonnet-4-6", VertexProject: "test-project", VertexLocation: "us-central1", TLSCaFile: caPath, TLSCertFile: certPath, TLSKeyFile: keyPath}

		chain, err := buildTransportChain(merged)
		Expect(err).NotTo(HaveOccurred())
		Expect(chain).NotTo(BeNil())
	})
})
