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

package readiness_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/readiness"
)

// fakeMCPClient is a test double for readiness.MCPClient.
type fakeMCPClient struct {
	ready        bool
	reconnectErr error
	reconnectN   int
}

func (f *fakeMCPClient) Ready() bool { return f.ready }

func (f *fakeMCPClient) Reconnect(_ context.Context) error {
	f.reconnectN++
	if f.reconnectErr == nil {
		f.ready = true
	}
	return f.reconnectErr
}

var _ = Describe("MCPClientProber", func() {
	It("UT-FLEET-READY-011: Probe succeeds without reconnecting when the client is already ready", func() {
		c := &fakeMCPClient{ready: true}
		p := &readiness.MCPClientProber{Client: c}
		Expect(p.Probe(context.Background())).To(Succeed())
		Expect(c.reconnectN).To(Equal(0))
	})

	It("UT-FLEET-READY-012: Probe attempts a reconnect when the client is not ready, and succeeds when reconnect succeeds", func() {
		c := &fakeMCPClient{ready: false}
		p := &readiness.MCPClientProber{Client: c}
		Expect(p.Probe(context.Background())).To(Succeed())
		Expect(c.reconnectN).To(Equal(1))
	})

	It("UT-FLEET-READY-013: Probe fails when the client is not ready and reconnect fails", func() {
		c := &fakeMCPClient{ready: false, reconnectErr: errors.New("dial tcp: connection refused")}
		p := &readiness.MCPClientProber{Client: c}
		err := p.Probe(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("connection refused"))
	})
})

// fakeClusterRegistry is a test double for readiness.ClusterRegistry.
type fakeClusterRegistry struct{ ready bool }

func (f *fakeClusterRegistry) Ready() bool { return f.ready }

var _ = Describe("ClusterRegistryProber", func() {
	It("UT-FLEET-READY-014: Probe succeeds when the registry is ready", func() {
		p := &readiness.ClusterRegistryProber{Registry: &fakeClusterRegistry{ready: true}}
		Expect(p.Probe(context.Background())).To(Succeed())
	})

	It("UT-FLEET-READY-015: Probe fails when the registry is not ready", func() {
		p := &readiness.ClusterRegistryProber{Registry: &fakeClusterRegistry{ready: false}}
		Expect(p.Probe(context.Background())).To(HaveOccurred())
	})
})

// fakePinger is a test double for readiness.Pinger.
type fakePinger struct{ err error }

func (f *fakePinger) Ping(_ context.Context) error { return f.err }

var _ = Describe("ScopeCheckerProber", func() {
	It("UT-FLEET-READY-016: Probe succeeds when Ping succeeds", func() {
		p := &readiness.ScopeCheckerProber{Pinger: &fakePinger{}}
		Expect(p.Probe(context.Background())).To(Succeed())
	})

	It("UT-FLEET-READY-017: Probe fails and wraps the underlying Ping error", func() {
		p := &readiness.ScopeCheckerProber{Pinger: &fakePinger{err: errors.New("connection refused")}}
		err := p.Probe(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("connection refused"))
	})
})
