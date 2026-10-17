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
	"errors"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

type bindingDefinitionDiscoverer struct {
	mu       sync.Mutex
	defs     map[string][]mcpclient.ToolDefinition
	entered  chan string
	returned chan string
}

func (d *bindingDefinitionDiscoverer) ListClusters(context.Context, string) ([]mcpclient.ClusterInfo, error) {
	return nil, nil
}

func (d *bindingDefinitionDiscoverer) ToolsForCluster(_ context.Context, clusterID string) ([]mcpclient.ToolDefinition, error) {
	if d.entered != nil {
		d.entered <- clusterID
	}
	d.mu.Lock()
	defs := append([]mcpclient.ToolDefinition(nil), d.defs[clusterID]...)
	d.mu.Unlock()
	if len(defs) == 0 {
		return nil, errors.New("cluster definitions unavailable")
	}
	if d.returned != nil {
		d.returned <- clusterID
	}
	return defs, nil
}

type bindingRecordingSession struct {
	mu    sync.Mutex
	names []string
}

func (s *bindingRecordingSession) CallTool(_ context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	s.names = append(s.names, params.Name)
	s.mu.Unlock()
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
}

func (s *bindingRecordingSession) callNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.names...)
}

var _ = Describe("Gateway binding leases (Issue #2518)", func() {
	It("UT-FLEET-2518-004 [AC-4, SI-10]: EAIGW resolves trusted prefixed bindings and overwrites caller wire names", func() {
		session := &bindingRecordingSession{}
		discoverer := &bindingDefinitionDiscoverer{defs: map[string][]mcpclient.ToolDefinition{
			"prod-east": {{Name: "prod-east__resources_get"}},
		}}
		resolver, err := mcpclient.NewGatewayBindingResolver(registry.GatewayEAIGW, discoverer)
		Expect(err).ToNot(HaveOccurred())

		lease, err := resolver.Resolve(context.Background(), mcpclient.SessionHandle{
			Session: session, Generation: 7,
		}, mcpclient.TargetRef{ClusterID: "prod-east"}, "resources_get")
		Expect(err).ToNot(HaveOccurred())
		Expect(lease.Binding().WireName).To(Equal("prod-east__resources_get"))
		Expect(lease.Binding().ReplayPolicy).To(Equal(mcpclient.ReplayRead))

		_, err = lease.CallTool(context.Background(), &mcp.CallToolParams{Name: "attacker__resources_delete"})
		Expect(err).ToNot(HaveOccurred())
		lease.Release()
		lease.Release()
		Expect(session.callNames()).To(Equal([]string{"prod-east__resources_get"}))
	})

	It("UT-FLEET-2518-004 [AC-4]: Kuadrant holds selection isolation through ListTools and CallTool", func() {
		session := &bindingRecordingSession{}
		discoverer := &bindingDefinitionDiscoverer{defs: map[string][]mcpclient.ToolDefinition{
			"prod-east": {{Name: "prod_east_resources_get"}},
			"prod-west": {{Name: "prod_west_resources_get"}},
		}, entered: make(chan string, 2), returned: make(chan string, 2)}
		resolver, err := mcpclient.NewGatewayBindingResolver(registry.GatewayKuadrant, discoverer)
		Expect(err).ToNot(HaveOccurred())

		leaseA, err := resolver.Resolve(context.Background(), mcpclient.SessionHandle{Session: session, Generation: 1}, mcpclient.TargetRef{ClusterID: "prod-east"}, "resources_get")
		Expect(err).ToNot(HaveOccurred())

		bDone := make(chan struct{})
		var leaseB mcpclient.ToolBindingLease
		var errB error
		go func() {
			leaseB, errB = resolver.Resolve(context.Background(), mcpclient.SessionHandle{Session: session, Generation: 1}, mcpclient.TargetRef{ClusterID: "prod-west"}, "resources_get")
			close(bDone)
		}()

		Consistently(bDone, 100*time.Millisecond, 10*time.Millisecond).ShouldNot(BeClosed(),
			"target B must not select the shared Kuadrant session while target A owns the lease")
		_, err = leaseA.CallTool(context.Background(), &mcp.CallToolParams{Name: "wrong"})
		Expect(err).ToNot(HaveOccurred())
		leaseA.Release()

		Eventually(bDone, time.Second, 10*time.Millisecond).Should(BeClosed())
		Expect(errB).ToNot(HaveOccurred())
		_, err = leaseB.CallTool(context.Background(), &mcp.CallToolParams{Name: "wrong"})
		Expect(err).ToNot(HaveOccurred())
		leaseB.Release()
		Expect(session.callNames()).To(Equal([]string{"prod_east_resources_get", "prod_west_resources_get"}))
	})

	It("UT-FLEET-2518-004 [SI-10]: stale refresh cannot publish behind a newer binding generation", func() {
		session := &bindingRecordingSession{}
		discoverer := &bindingDefinitionDiscoverer{defs: map[string][]mcpclient.ToolDefinition{
			"prod-east": {{Name: "prod-east__resources_get"}},
		}}
		resolver, err := mcpclient.NewGatewayBindingResolver(registry.GatewayEAIGW, discoverer)
		Expect(err).ToNot(HaveOccurred())
		handle := mcpclient.SessionHandle{Session: session, Generation: 1}
		target := mcpclient.TargetRef{ClusterID: "prod-east"}

		first, err := resolver.Resolve(context.Background(), handle, target, "resources_get")
		Expect(err).ToNot(HaveOccurred())
		first.Release()
		second, err := resolver.Resolve(context.Background(), handle, target, "resources_get")
		Expect(err).ToNot(HaveOccurred())
		defer second.Release()

		_, err = resolver.Refresh(context.Background(), handle, target, "resources_get", first.Binding().BindingGeneration)
		Expect(err).To(MatchError(mcpclient.ErrStaleBinding))
	})
})
