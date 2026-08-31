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

package fleet_test

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/readiness"
	infrastructure "github.com/jordigilh/kubernaut/test/infrastructure"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

var _ = Describe("ReaderFactory (BR-INTEGRATION-054)", func() {
	var (
		ctx         context.Context
		localClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme := runtime.NewScheme()
		localClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	})

	Describe("LocalReaderFactory", func() {
		It("UT-FLEET-RF-001: returns local client for empty ClusterID", func() {
			factory := fleet.NewLocalReaderFactory(localClient)
			reader, err := factory.ReaderFor(ctx, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(reader).To(Equal(localClient))
		})

		It("UT-FLEET-RF-002: returns local client for any ClusterID (local-only mode)", func() {
			factory := fleet.NewLocalReaderFactory(localClient)
			reader, err := factory.ReaderFor(ctx, "remote-cluster-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(reader).To(Equal(localClient),
				"local-only factory always returns local client regardless of clusterID")
		})
	})

	Describe("MCPReaderFactory", func() {
		It("UT-FLEET-RF-003: returns local client for empty ClusterID", func() {
			factory := mcpclient.NewMCPReaderFactory(localClient, nil)
			reader, err := factory.ReaderFor(ctx, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(reader).To(Equal(localClient))
		})

		It("UT-FLEET-RF-004: returns error for remote ClusterID when session is nil", func() {
			factory := mcpclient.NewMCPReaderFactory(localClient, nil)
			_, err := factory.ReaderFor(ctx, "remote-cluster-1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("MCP session not available"))
		})
	})

	Describe("MCPReaderFactory self-healing after an initial connect failure (issue #2315)", func() {
		// Representative test for every reader-factory-based fleet-aware
		// service (gateway, signalprocessing, remediationorchestrator,
		// effectivenessmonitor, apifrontend): they all end up calling
		// mcpclient.NewMCPReaderFactoryWithProvider(localClient,
		// resilientClient.SessionProvider()) once Connect() is wired
		// unconditionally, so proving self-healing here at the shared
		// factory level covers all of them without duplicating this IT
		// test five times.
		It("IT-FLEET-RF-006: the same ReaderFactory recovers once the gateway becomes reachable, with no restart", func() {
			gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("remote-cluster"))
			defer gw.Close()

			proxy, err := infrastructure.NewInterruptibleProxy(gw.URL()[len("http://"):])
			Expect(err).ToNot(HaveOccurred())
			defer proxy.Close()
			proxy.Pause()

			connectCfg := mcpclient.ConnectConfig{
				Endpoint: "http://" + proxy.Addr(),
				Resilience: fleet.FleetResilienceConfig{
					InitialInterval: 50 * time.Millisecond,
					MaxInterval:     100 * time.Millisecond,
					MaxElapsedTime:  500 * time.Millisecond,
					ConnectTimeout:  200 * time.Millisecond,
				},
			}

			rc, connErr := mcpclient.Connect(ctx, connectCfg, logr.Discard())
			Expect(rc).ToNot(BeNil())
			Expect(connErr).To(HaveOccurred(), "must start disconnected: the proxy is paused")
			defer func() { _ = rc.Close() }()

			factory := mcpclient.NewMCPReaderFactoryWithProvider(localClient, rc.SessionProvider(), rc.Reconnect)

			_, err = factory.ReaderFor(ctx, "remote-cluster")
			Expect(err).To(HaveOccurred(), "must fail while the gateway is unreachable through the paused proxy")

			// Mirrors production wiring (wireFleetReadinessGate /
			// MCPClientProber): the periodic probe is what actually
			// drives ResilientClient's background reconnect.
			gate := readiness.NewGate(100*time.Millisecond, GinkgoLogr, &readiness.MCPClientProber{Client: rc})
			gate.Start(ctx)
			defer gate.Stop()

			proxy.Resume()

			Eventually(func() error {
				reader, err := factory.ReaderFor(ctx, "remote-cluster")
				if err != nil {
					return err
				}
				if reader == nil {
					return fmt.Errorf("expected a non-nil reader")
				}
				return nil
			}, 10*time.Second, 100*time.Millisecond).Should(Succeed(),
				"issue #2315: the SAME ReaderFactory returned at startup must succeed once the gateway "+
					"becomes reachable, with no restart")
		})
	})

	Describe("ReaderFactoryFunc", func() {
		It("UT-FLEET-RF-005: adapts a plain function to the ReaderFactory interface", func() {
			called := false
			fn := fleet.ReaderFactoryFunc(func(_ context.Context, clusterID string) (client.Reader, error) {
				called = true
				Expect(clusterID).To(Equal("test-cluster"))
				return localClient, nil
			})
			reader, err := fn.ReaderFor(ctx, "test-cluster")
			Expect(err).ToNot(HaveOccurred())
			Expect(reader).To(Equal(localClient))
			Expect(called).To(BeTrue())
		})
	})

	// =========================================================================
	// Mid-lifetime session death (issue #2317)
	// =========================================================================
	// Representative for every reader-factory-based fleet-aware service
	// (gateway, signalprocessing, remediationorchestrator,
	// effectivenessmonitor, apifrontend): unlike IT-FLEET-RF-006 above (which
	// covers an *initial* connect failure), this proves the per-cluster
	// Client returned by ReaderFor recovers from a session that dies mid-
	// lifetime -- e.g. an SSE stream failure between two Get calls, broker
	// healthy, client session dead -- because ReaderFor now wires the
	// reconnect callback through as WithReconnect.
	Describe("MCPReaderFactory reconnect-on-failure (issue #2317)", func() {
		It("UT-FLEET-RF-007: the reader returned by ReaderFor recovers via reconnect after the session dies", func() {
			gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))
			defer gw.Close()

			cfg := mcpclient.DefaultResilienceConfig()
			cfg.MaxElapsedTime = 5 * time.Second
			rc, err := mcpclient.NewResilient(ctx, gw.URL(), cfg, logr.Discard())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = rc.Close() }()

			factory := mcpclient.NewMCPReaderFactoryWithProvider(localClient, rc.SessionProvider(), rc.Reconnect)

			reader, err := factory.ReaderFor(ctx, "prod-east")
			Expect(err).ToNot(HaveOccurred())

			// Simulate the session dying from a protocol-level error while
			// the Gateway itself stays healthy.
			Expect(rc.Session().Close()).ToNot(HaveOccurred())

			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Pod"})
			err = reader.Get(ctx, client.ObjectKey{Namespace: "default", Name: "nginx"}, obj)
			Expect(err).ToNot(HaveOccurred(),
				"Get must recover by reconnecting once the session is dead, not fail forever")
		})
	})
})
