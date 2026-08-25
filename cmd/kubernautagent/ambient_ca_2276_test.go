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
	"os"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// Issue #2276: KubernautAgent's primary MCP Gateway connection (and any
// other outbound client in this process relying on the system trust store)
// had no way to be told about the deployer's inter-service CA except via a
// static Pod-spec env: TLS_CA_FILE entry -- the same declaration pattern
// that produced a duplicate-env-var-name bug in kubernaut-operator.
// bootstrapAmbientCATrust moves that CA source into the resolved YAML
// config instead.
var _ = Describe("cmd/kubernautagent bootstrapAmbientCATrust — ambient CA trust wiring (#2276, SC-8)", func() {

	AfterEach(func() {
		Expect(os.Unsetenv("SSL_CERT_FILE")).To(Succeed())
		Expect(os.Unsetenv("TLS_CA_FILE")).To(Succeed())
		sharedtls.ResetSystemCertFileCandidatesForTesting()
	})

	Describe("IT-KA-2276-001: a tlsCaFile configured via YAML is wired through bootstrapAmbientCATrust into ambient env vars", func() {
		It("parses runtime.server.tlsCaFile from YAML and injects it via the same bootstrapAmbientCATrust call main() makes", func() {
			realCA := generateTestCACert(GinkgoTB(), "ka-ambient-ca-2276-test")
			sharedtls.SetSystemCertFileCandidatesForTesting([]string{"/nonexistent/system-bundle.pem"})

			cfgYAML := []byte(`
runtime:
  server:
    tlsCaFile: ` + realCA + `
`)
			cfg, err := kaconfig.Load(cfgYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Runtime.Server.TLSCAFile).To(Equal(realCA),
				"kaconfig.Load must parse runtime.server.tlsCaFile into ServerConfig.TLSCAFile")

			// bootstrapAmbientCATrust is the exact production function
			// main() calls immediately after loadStartupConfig, before
			// telemetry.Bootstrap's OTel exporter.
			Expect(bootstrapAmbientCATrust(logr.Discard(), cfg)).To(Succeed())

			Expect(os.Getenv("TLS_CA_FILE")).To(Equal(realCA))
			Expect(os.Getenv("SSL_CERT_FILE")).NotTo(BeEmpty())
		})
	})

	Describe("IT-KA-2276-002: an unset tlsCaFile leaves ambient env vars untouched (fail-open parity)", func() {
		It("does not set SSL_CERT_FILE or TLS_CA_FILE when tlsCaFile is absent from config", func() {
			cfg, err := kaconfig.Load([]byte(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Runtime.Server.TLSCAFile).To(BeEmpty())

			Expect(bootstrapAmbientCATrust(logr.Discard(), cfg)).To(Succeed())

			Expect(os.Getenv("SSL_CERT_FILE")).To(BeEmpty())
			Expect(os.Getenv("TLS_CA_FILE")).To(BeEmpty())
		})
	})
})
