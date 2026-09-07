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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("certificateSignedByCA", func() {
	It("UT-INFRA-FLEETDEMO-047: accepts a leaf signed by the current CA", func() {
		ca, leafPEM, setupErr := testCertificateChain()

		Expect(setupErr).NotTo(HaveOccurred())
		matches, err := certificateSignedByCA(leafPEM, ca)

		Expect(err).NotTo(HaveOccurred())
		Expect(matches).To(BeTrue())
	})

	It("UT-INFRA-FLEETDEMO-048: rejects a leaf signed by an obsolete CA", func() {
		currentCA, _, currentCAErr := testCertificateChain()
		obsoleteCA, leafPEM, obsoleteCAErr := testCertificateChain()

		Expect(currentCAErr).NotTo(HaveOccurred())
		Expect(obsoleteCAErr).NotTo(HaveOccurred())
		matches, err := certificateSignedByCA(leafPEM, currentCA)

		Expect(err).NotTo(HaveOccurred())
		Expect(matches).To(BeFalse())
		Expect(obsoleteCA).NotTo(BeNil())
	})

	It("UT-INFRA-FLEETDEMO-049: rejects malformed certificate PEM", func() {
		ca, _, setupErr := testCertificateChain()

		Expect(setupErr).NotTo(HaveOccurred())
		matches, err := certificateSignedByCA([]byte("not-a-certificate"), ca)

		Expect(err).To(MatchError("certificate PEM block not found"))
		Expect(matches).To(BeFalse())
	})
})

func testCertificateChain() (*x509.Certificate, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, nil, err
	}

	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "keycloak"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		DNSNames:     []string{"localhost"},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, ca, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	return ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), nil
}
