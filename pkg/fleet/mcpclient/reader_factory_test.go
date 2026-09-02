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
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// Mid-lifetime session death for the tool-prefix discovery path used by
// mcpReaderFactory.ReaderFor (issue #2340, a gap #2317 missed).
//
// #2317 added reconnect-on-retryable-error to providerDiscoverer's
// ListClusters/ToolsForCluster (discovery.go) and to Client.callTool itself,
// but mcpReaderFactory.ReaderFor's own tool-prefix lookup -- the
// DiscoverToolPrefix(ctx, session, clusterID) call taken when no
// registry.ToolPrefixResolver is configured, which is exactly how
// cmd/gateway/main.go wires NewMCPReaderFactoryWithProvider today -- calls
// DiscoverToolPrefix directly on the raw resolved session, bypassing the
// same self-healing retry even though f.reconnect is available on the
// struct. Observed live on the Fleet E2E hub: Gateway's Prometheus adapter
// permanently failed remote owner-chain resolution for "remote-cluster"
// with "list tools from MCP Gateway: connection closed: ... standalone SSE
// stream: exceeded 5 retries without progress" after the underlying session
// died mid-lifetime, even though the MCP Gateway and kube-mcp-server both
// stayed healthy throughout.
var _ = Describe("mcpReaderFactory.ReaderFor reconnect-on-failure (issue #2317 gap)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-READERFACTORY-RECONN-001: ReaderFor recovers via reconnect after the underlying session dies mid-lifetime", func() {
		gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))
		defer gw.Close()

		cfg := mcpclient.DefaultResilienceConfig()
		cfg.MaxElapsedTime = 5 * time.Second
		rc, err := mcpclient.NewResilient(ctx, gw.URL(), cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = rc.Close() }()

		factory := mcpclient.NewMCPReaderFactoryWithProvider(nil, rc.SessionProvider(), rc.Reconnect)

		// Simulate the session dying from a protocol-level error while the
		// MCP Gateway itself stays healthy -- exactly the observed
		// production scenario (gateway healthy, client session dead).
		Expect(rc.Session().Close()).ToNot(HaveOccurred())

		reader, err := factory.ReaderFor(ctx, "prod-east")
		Expect(err).ToNot(HaveOccurred(),
			"ReaderFor must recover by reconnecting once the session is dead, not fail forever")
		Expect(reader).ToNot(BeNil())

		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "PodList"})
		Expect(reader.List(ctx, list, client.InNamespace("default"))).To(Succeed(),
			"the reader returned after reconnect must actually be usable")
	})

	It("UT-READERFACTORY-RECONN-002: without a reconnect callback, a dead session fails ReaderFor every time", func() {
		gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-west"))
		defer gw.Close()

		cfg := mcpclient.DefaultResilienceConfig()
		cfg.MaxElapsedTime = 5 * time.Second
		rc, err := mcpclient.NewResilient(ctx, gw.URL(), cfg, logr.Discard())
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = rc.Close() }()

		factory := mcpclient.NewMCPReaderFactoryWithProvider(nil, rc.SessionProvider(), nil)

		Expect(rc.Session().Close()).ToNot(HaveOccurred())

		_, err = factory.ReaderFor(ctx, "prod-west")
		Expect(err).To(HaveOccurred(), "without a reconnect callback there is no way to repair a dead session")
	})
})
