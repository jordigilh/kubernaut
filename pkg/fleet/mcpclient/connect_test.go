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

package mcpclient_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

var _ = Describe("Connect (issue #2315 self-healing fix)", func() {
	var logger = zap.New(zap.UseDevMode(true))

	It("UT-FLEET-CONN-001: always returns a non-nil client even when the initial connect fails", func() {
		cfg := mcpclient.ConnectConfig{
			Endpoint: "http://127.0.0.1:1/mcp",
			Resilience: fleet.FleetResilienceConfig{
				InitialInterval: 50 * time.Millisecond,
				MaxInterval:     100 * time.Millisecond,
				MaxElapsedTime:  300 * time.Millisecond,
			},
		}

		rc, err := mcpclient.Connect(context.Background(), cfg, logger)
		Expect(rc).ToNot(BeNil(),
			"Connect must always return a non-nil *ResilientClient when Endpoint is set, "+
				"regardless of whether the initial connect attempt succeeded -- callers must "+
				"not gate dependent resolver/factory construction on the returned error")
		Expect(err).To(HaveOccurred())
		Expect(rc.Ready()).To(BeFalse())
	})

	It("UT-FLEET-CONN-002: succeeds against a reachable gateway with no OAuth2 configured", func() {
		gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("c1"))
		defer gw.Close()

		cfg := mcpclient.ConnectConfig{
			Endpoint: gw.URL(),
		}

		rc, err := mcpclient.Connect(context.Background(), cfg, logger)
		Expect(err).ToNot(HaveOccurred())
		Expect(rc).ToNot(BeNil())
		Expect(rc.Ready()).To(BeTrue())
		defer func() { _ = rc.Close() }()
	})

	It("UT-FLEET-CONN-003: wires a reloadable OAuth2 transport when OAuth2.Enabled is true", func() {
		tmpDir, err := os.MkdirTemp("", "connect-oauth2-test")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = os.RemoveAll(tmpDir) }()

		Expect(os.WriteFile(filepath.Join(tmpDir, "client-id"), []byte("id"), 0o600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tmpDir, "client-secret"), []byte("secret"), 0o600)).To(Succeed())

		gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("c1"))
		defer gw.Close()

		cfg := mcpclient.ConnectConfig{
			Endpoint: gw.URL(),
			OAuth2: fleet.FleetOAuth2Config{
				Enabled:  true,
				TokenURL: "http://127.0.0.1:1/token",
			},
			CredentialsBasePath: tmpDir,
			Resilience: fleet.FleetResilienceConfig{
				InitialInterval: 50 * time.Millisecond,
				MaxInterval:     100 * time.Millisecond,
				MaxElapsedTime:  300 * time.Millisecond,
			},
		}

		// The OAuth2 transport is lazy (no token fetched at construction
		// time), but every connect attempt against the reachable gateway
		// still fails because the unreachable token endpoint can't supply a
		// bearer token -- proving the transport was actually wired into the
		// connect path, not silently skipped.
		rc, err := mcpclient.Connect(context.Background(), cfg, logger)
		Expect(rc).ToNot(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(rc.Ready()).To(BeFalse())
	})
})
