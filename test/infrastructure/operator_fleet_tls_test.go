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

package infrastructure

import (
	"crypto/x509"
	"encoding/pem"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("operator Fleet TLS material", func() {
	It("UT-INFRA-FLEET-TLS-005: signs the HTTPS gateway leaf from the operator CA and covers both gateway service identities", func() {
		now := time.Date(2026, time.October, 21, 12, 0, 0, 0, time.UTC)
		caPEM, caKeyPEM, err := generateQualificationCA(now)
		Expect(err).NotTo(HaveOccurred())
		leafPEM, _, err := signQualificationLeaf(caPEM, caKeyPEM, now)
		Expect(err).NotTo(HaveOccurred())

		caBlock, _ := pem.Decode(caPEM)
		leafBlock, _ := pem.Decode(leafPEM)
		Expect(caBlock).NotTo(BeNil())
		Expect(leafBlock).NotTo(BeNil())
		ca, err := x509.ParseCertificate(caBlock.Bytes)
		Expect(err).NotTo(HaveOccurred())
		leaf, err := x509.ParseCertificate(leafBlock.Bytes)
		Expect(err).NotTo(HaveOccurred())

		roots := x509.NewCertPool()
		roots.AddCert(ca)
		for _, name := range []string{
			"localhost",
			"mcp-gateway-istio.gateway-system.svc",
			"generated.envoy-gateway-system.svc.cluster.local",
			"kube-mcp-server.127-0-0-1.sslip.io",
			"kube-mcp-server-remote.127-0-0-1.sslip.io",
		} {
			_, err = leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: name, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
			Expect(err).NotTo(HaveOccurred(), "certificate must verify for %s", name)
		}
		_, err = leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "127.0.0.1", KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
		Expect(err).NotTo(HaveOccurred())
	})

	It("UT-INFRA-FLEET-TLS-006: keeps the operator CA private key separate from the serving certificate", func() {
		now := time.Date(2026, time.October, 21, 12, 0, 0, 0, time.UTC)
		caPEM, caKeyPEM, err := generateQualificationCA(now)
		Expect(err).NotTo(HaveOccurred())
		leafPEM, leafKeyPEM, err := signQualificationLeaf(caPEM, caKeyPEM, now)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(caPEM)).NotTo(ContainSubstring(string(leafKeyPEM)))
		Expect(string(caPEM)).NotTo(ContainSubstring(string(leafPEM)))
		Expect(string(caKeyPEM)).NotTo(ContainSubstring(string(leafPEM)))
	})
})
