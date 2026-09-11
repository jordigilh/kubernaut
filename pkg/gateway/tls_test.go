package gateway_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/cert"
)

// gatewayTestCertDir provides the same certificate material that a chart
// Secret mounts in production. Gateway tests must exercise the HTTPS API path,
// not bypass it with an implicit plaintext listener.
func gatewayTestCertDir() string {
	dir := GinkgoT().TempDir()
	pair, err := cert.GenerateSelfSigned(cert.CertificateOptions{
		CommonName: "localhost",
		DNSNames:   []string{"localhost"},
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(os.WriteFile(filepath.Join(dir, "tls.crt"), pair.CertPEM, 0o600)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(dir, "tls.key"), pair.KeyPEM, 0o600)).To(Succeed())
	return dir
}
