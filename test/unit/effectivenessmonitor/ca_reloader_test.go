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
	"crypto/tls"
	"net/http"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
)

var _ = Describe("CAReloader (Issue #484, BR-PLATFORM-452)", Label("tls", "hotreload", "484"), func() {

	// ========================================
	// UT-EM-484-001: NewCAReloader succeeds with valid PEM
	// ========================================
	It("UT-EM-484-001: NewCAReloader initializes cert pool from valid PEM data", func() {
		caPEM := generateTestCACert()

		reloader, err := emclient.NewCAReloader(caPEM)
		Expect(err).NotTo(HaveOccurred())
		Expect(reloader).NotTo(BeNil())
		Expect(reloader.GetCertPool()).NotTo(BeNil())
	})

	// ========================================
	// UT-EM-484-002: NewCAReloader rejects invalid PEM
	// ========================================
	It("UT-EM-484-002: NewCAReloader returns error for invalid PEM data", func() {
		reloader, err := emclient.NewCAReloader([]byte("not-valid-pem"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no valid PEM"))
		Expect(reloader).To(BeNil())
	})

	// ========================================
	// UT-EM-484-003: ReloadCallback swaps transport atomically
	// ========================================
	It("UT-EM-484-003: ReloadCallback replaces the underlying transport with new CA", func() {
		caPEM := generateTestCACert()
		reloader, err := emclient.NewCAReloader(caPEM)
		Expect(err).NotTo(HaveOccurred())

		poolBefore := reloader.GetCertPool()
		Expect(poolBefore).NotTo(BeNil())

		newCAPEM := generateTestCACert()
		err = reloader.ReloadCallback(string(newCAPEM))
		Expect(err).NotTo(HaveOccurred())

		poolAfter := reloader.GetCertPool()
		Expect(poolAfter).NotTo(BeNil())
		Expect(poolAfter).NotTo(BeIdenticalTo(poolBefore), "cert pool should be replaced, not mutated")
	})

	// ========================================
	// UT-EM-484-004: ReloadCallback rejects invalid PEM, keeps previous
	// ========================================
	It("UT-EM-484-004: ReloadCallback rejects invalid PEM and preserves previous cert pool", func() {
		caPEM := generateTestCACert()
		reloader, err := emclient.NewCAReloader(caPEM)
		Expect(err).NotTo(HaveOccurred())

		poolBefore := reloader.GetCertPool()

		err = reloader.ReloadCallback("garbage-pem-data")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no valid PEM"))

		poolAfter := reloader.GetCertPool()
		Expect(poolAfter).To(BeIdenticalTo(poolBefore), "cert pool should be unchanged after failed reload")
	})

	// ========================================
	// UT-EM-484-005: RoundTrip delegates to a transport using the current cert pool
	// ========================================
	It("UT-EM-484-005: RoundTrip uses a transport configured with the current cert pool", func() {
		caPEM := generateTestCACert()
		reloader, err := emclient.NewCAReloader(caPEM)
		Expect(err).NotTo(HaveOccurred())

		var rt http.RoundTripper = reloader
		Expect(rt).NotTo(BeNil(), "CAReloader must implement http.RoundTripper")

		innerTransport := reloader.CurrentTransport()
		Expect(innerTransport).NotTo(BeNil())
		Expect(innerTransport.TLSClientConfig).NotTo(BeNil())
		Expect(innerTransport.TLSClientConfig.MinVersion).To(Equal(uint16(tls.VersionTLS12)))
		Expect(innerTransport.TLSClientConfig.RootCAs).NotTo(BeNil())
	})

	// ========================================
	// UT-EM-484-006: Concurrent reload + read is safe
	// ========================================
	It("UT-EM-484-006: concurrent ReloadCallback and GetCertPool calls are safe", func() {
		caPEM := generateTestCACert()
		reloader, err := emclient.NewCAReloader(caPEM)
		Expect(err).NotTo(HaveOccurred())

		var wg sync.WaitGroup
		const goroutines = 20

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer GinkgoRecover()
				newPEM := generateTestCACert()
				_ = reloader.ReloadCallback(string(newPEM))
			}()
		}

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer GinkgoRecover()
				pool := reloader.GetCertPool()
				Expect(pool).NotTo(BeNil())
			}()
		}

		wg.Wait()
	})
})
