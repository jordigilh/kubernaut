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
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-logr/zapr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	fmcconfig "github.com/jordigilh/kubernaut/pkg/fleet/fmc/config"
)

// Issue #2694 (BR-SECURITY-016, FedRAMP AU-2): FleetMetadataCache's
// buildFMCServers must route the shared auth middleware's security_event
// records onto the dedicated unsampled, non-level-gated logger derived from
// the host logger.
//
// The host logger is a real zap core at INFO level writing to an in-memory
// buffer. The level matters for discrimination: if the wiring were missing,
// emitSecurityEvent's fallback would write the Info security_event record
// into this host buffer and the spec would fail. With the wiring in place
// the record goes to the dedicated logger instead. The dedicated logger's
// WARN-gate/sampler guarantees are proven in pkg/log (UT-LOG-2694-002/003)
// and pkg/shared/auth (UT-AUTH-2694-003/004).

var _ = Describe("FMC auth security_event dedicated logger wiring [Issue #2694]", func() {
	It("IT-FMC-2694-001 [BR-SECURITY-016, AU-2]: buildFMCServers routes security events off the host logger", func() {
		certDir := GinkgoT().TempDir()
		generateSelfSignedCert(filepath.Join(certDir, "tls.crt"), filepath.Join(certDir, "tls.key"))

		cfg := fmcconfig.DefaultServiceConfig()
		cfg.Server.APIAddr = osAssignedAddr
		cfg.Server.HealthAddr = osAssignedAddr
		cfg.Server.MetricsAddr = osAssignedAddr
		cfg.Server.TLS.CertDir = certDir

		hostBuf := &syncBufferFMC{}
		hostLogger := zapr.NewLogger(zap.New(zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), hostBuf, zapcore.InfoLevel)))

		servers := buildFMCServers(cfg, testFMCDeps(fakeAuthorizedK8sClient(), unreachableValkeyAddr), hostLogger)

		ln, addr := listenOn()
		serveErr := make(chan error, 1)
		go func() { serveErr <- servers.api.ServeTLS(ln, "", "") }()

		// No bearer token: rejected at the missing Authorization header,
		// before any TokenReview/SAR round-trip, so the fake K8s client is
		// never consulted.
		client := &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: caPoolFromCert(filepath.Join(certDir, "tls.crt"))}, //nolint:gosec // test CA
		}}
		resp, err := client.Get("https://" + addr + "/api/v1/clusters") //nolint:noctx // test-only probe
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

		// Shut the server down explicitly, then collect ServeTLS's result:
		// the probe succeeded, so it can only report the deliberate
		// shutdown; anything else is the probe's real failure cause.
		Expect(servers.api.Close()).To(Succeed())
		Expect(<-serveErr).To(SatisfyAny(
			BeNil(),
			MatchError(http.ErrServerClosed),
		), "ServeTLS must serve the probe and then shut down cleanly")

		var securityEvents int
		for _, line := range strings.Split(strings.TrimSpace(hostBuf.String()), "\n") {
			if line == "" {
				continue
			}
			var entry map[string]any
			Expect(json.Unmarshal([]byte(line), &entry)).To(Succeed())
			if entry["msg"] == "security_event" {
				securityEvents++
			}
		}
		Expect(securityEvents).To(Equal(0),
			"the security_event record must leave the host logger's level-gated path: "+
				"buildFMCServers wires the dedicated unsampled security-event logger")
	})
})

// syncBufferFMC is a concurrency-safe zapcore.WriteSyncer.
type syncBufferFMC struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBufferFMC) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBufferFMC) Sync() error { return nil }

func (b *syncBufferFMC) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
