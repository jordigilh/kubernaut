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

package executor_test

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// UT-WE-054-CF: ClientFactory unit tests
// Authority: BR-FLEET-054 (Fleet Multi-Cluster Execution)
// FedRAMP: AC-3 (Access Enforcement) -- local factory rejects remote clusters
var _ = Describe("UT-WE-054-CF: ClientFactory", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	Context("LocalClientFactory", func() {
		It("UT-WE-054-CF-001: should return local client for empty clusterID", func() {
			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := executor.NewLocalClientFactory(localClient)

			client, err := factory.ClientFor(ctx, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
		})

		It("UT-WE-054-CF-002: should reject non-empty clusterID when fleet not configured", func() {
			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := executor.NewLocalClientFactory(localClient)

			_, err := factory.ClientFor(ctx, "prod-east")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("remote execution not configured"))
			Expect(err.Error()).To(ContainSubstring("prod-east"))
		})
	})

	Context("MCPClientFactory", func() {
		It("UT-WE-054-CF-003: should return local client for empty clusterID", func() {
			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := executor.NewMCPClientFactory(localClient, nil)

			client, err := factory.ClientFor(ctx, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
		})

		It("UT-WE-054-CF-004: returns a clean error (not a panic) when session is nil for remote clusterID (issue #2315)", func() {
			// Issue #2315: a nil session for a remote clusterID must be a
			// clean, typed error, not a panic -- once callers build this
			// factory unconditionally (even when the initial MCP Gateway
			// connect attempt failed), a nil/not-yet-connected session is
			// an expected, self-healing-pending runtime state, not a
			// programmer error.
			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := executor.NewMCPClientFactory(localClient, nil)

			_, err := factory.ClientFor(ctx, "prod-west")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("prod-west"))
			Expect(err.Error()).To(ContainSubstring("MCP session not available"))
		})
	})

	Context("MCPClientFactoryWithProvider (issue #2315 self-healing fix)", func() {
		It("UT-WE-054-CF-005: returns local client for empty clusterID", func() {
			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := executor.NewMCPClientFactoryWithProvider(localClient, func() *mcp.ClientSession { return nil }, nil)

			client, err := factory.ClientFor(ctx, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
		})

		It("UT-WE-054-CF-006: returns a clean error for a remote clusterID while the provider reports disconnected", func() {
			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := executor.NewMCPClientFactoryWithProvider(localClient, func() *mcp.ClientSession { return nil }, nil)

			_, err := factory.ClientFor(ctx, "prod-west")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("prod-west"))
			Expect(err.Error()).To(ContainSubstring("MCP session not available"))
		})
	})

	// =========================================================================
	// Mid-lifetime session death (issue #2317)
	// =========================================================================
	// mcpClientFactory.ClientFor previously built its returned remoteClient
	// via NewFromSession/NewWriterFromSession -- a fixed session snapshot
	// with no reconnect callback, even in the provider-based construction
	// path. A session that died from a protocol-level error between two
	// Get/Create calls on the same returned client (broker healthy, client
	// session dead) failed every subsequent call forever, exactly the
	// failure class already fixed for KA's discoverer and the other
	// services' reader factories (issue #2315/#2317).
	Context("MCPClientFactoryWithProvider reconnect-on-failure (issue #2317)", func() {
		It("UT-WE-054-CF-007: ClientFor's returned client recovers via reconnect after the session dies mid-lifetime", func() {
			gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))
			defer gw.Close()

			cfg := mcpclient.DefaultResilienceConfig()
			cfg.MaxElapsedTime = 5 * time.Second
			rc, err := mcpclient.NewResilient(ctx, gw.URL(), cfg, logr.Discard())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = rc.Close() }()

			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := executor.NewMCPClientFactoryWithProvider(localClient, rc.SessionProvider(), rc.Reconnect)

			execClient, err := factory.ClientFor(ctx, "prod-east")
			Expect(err).ToNot(HaveOccurred())

			// Simulate the session dying from a protocol-level error while
			// the Gateway itself stays healthy.
			Expect(rc.Session().Close()).ToNot(HaveOccurred())

			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Pod"})
			err = execClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "nginx"}, obj)
			Expect(err).ToNot(HaveOccurred(),
				"Get must recover by reconnecting once the session is dead, not fail forever")
		})

		// UT-WE-054-CF-007 above only kills the session *after* ClientFor has
		// already returned a client -- it never exercises ClientFor's own
		// internal DiscoverToolPrefix call against an already-dead session.
		// Issue #2346 (a third instance of #2340's gap): ClientFor is called
		// fresh on every JobExecutor.Create/GetStatus (not cached), so a
		// session that dies *before* ClientFor runs -- e.g. between the
		// AIAnalysis phase and the WorkflowExecution phase -- hit this
		// exact, previously-uncovered path in production.
		It("UT-WE-054-CF-008: ClientFor itself recovers via reconnect when the session is already dead before the call (issue #2346)", func() {
			gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("remote-cluster"))
			defer gw.Close()

			cfg := mcpclient.DefaultResilienceConfig()
			cfg.MaxElapsedTime = 5 * time.Second
			rc, err := mcpclient.NewResilient(ctx, gw.URL(), cfg, logr.Discard())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = rc.Close() }()

			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := executor.NewMCPClientFactoryWithProvider(localClient, rc.SessionProvider(), rc.Reconnect)

			// Kill the session BEFORE calling ClientFor -- simulates the
			// production scenario where the gateway stays healthy but the
			// client's session died in between two ClientFor calls.
			Expect(rc.Session().Close()).ToNot(HaveOccurred())

			execClient, err := factory.ClientFor(ctx, "remote-cluster")
			Expect(err).ToNot(HaveOccurred(),
				"ClientFor must recover by reconnecting once the session is already dead, not fail forever")
			Expect(execClient).ToNot(BeNil())
		})
	})
})
