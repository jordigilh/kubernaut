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

package tls2548rendered

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/shared/tlscontract"
)

// IT-TLS-2548-001: the production deployment surface -- the operator chart
// rendered by CI -- must satisfy the same ambient CA deployment contract as
// the repository corpus: no statically injected SSL_CERT_FILE/SSL_CERT_DIR
// and no writable CA/TLS material mounts. TLS_CA_FILE stays allowed for
// explicit reloadable transports. FedRAMP SC-8/SC-13, AC-6/CM-7; SOC 2
// CC8.1; OWASP ASVS V12.3.1-V12.3.4, V15.2.5. [BR-SECURITY-2548]
var _ = Describe("rendered operator TLS contract [BR-SECURITY-2548]", func() {
	renderedDir := os.Getenv("TLS_2548_RENDERED_DIR")

	It("IT-TLS-2548-001: rendered operator bundles never inject ambient CA env or writable CA mounts", func() {
		// Fail closed: this suite exists to gate the CI-rendered
		// production contract; an unset input is an error, never a skip.
		emptyDir := ""
		ExpectWithOffset(1, renderedDir).
			ToNot(Equal(emptyDir), "TLS_2548_RENDERED_DIR must be set to the downloaded operator-qualification artifact directory")

		expectedBundles := []string{
			"operator-bootstrap-rendered.yaml",
			"operator-dependencies-rendered.yaml",
		}
		streams := map[string][]byte{}
		for _, bundle := range expectedBundles {
			path := filepath.Join(renderedDir, bundle)
			data, err := os.ReadFile(path) //nolint:gosec // G304: CI-provided artifact path
			ExpectWithOffset(1, err).ToNot(HaveOccurred(),
				fmt.Sprintf("rendered bundle %s must exist in %s", bundle, renderedDir))
			streams[path] = data
		}

		// Scan both bundles as one corpus: envFrom references must
		// resolve across the rendered deployment surface, not just
		// within a single bundle.
		result, err := tlscontract.ScanManifestCorpus(streams)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())

		ExpectWithOffset(1, result.HasViolations()).To(BeFalse(),
			"rendered operator contract violated for BR-SECURITY-2548:\n"+strings.Join(result.SortedViolations(), "\n"))
		// Anti-vacuity: the rendered operator bundle is a deployment
		// surface; it must contain workload documents to scan.
		ExpectWithOffset(1, result.WorkloadDocCount).To(BeNumerically(">", 0),
			"rendered operator bundles must contain workload documents for BR-SECURITY-2548")
	})
})
