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
	"io"
	"net"
	"sync"
)

// InterruptibleProxy is a transparent TCP proxy that forwards raw bytes between
// a local listener and a remote target. It tracks all active connections so they
// can be forcibly closed via DisconnectAll(), simulating a network partition.
//
// TLS termination happens at the target (KA), so the proxy never sees plaintext.
// The KA leaf cert includes "localhost" in its SANs, which matches regardless of
// the proxy's ephemeral port.
type InterruptibleProxy struct {
	listener net.Listener
	target   string
	mu       sync.Mutex
	conns    []net.Conn
	done     chan struct{}
}

// NewInterruptibleProxy creates a proxy listening on a random localhost port.
// All accepted connections are forwarded to target (e.g., "localhost:8088").
func NewInterruptibleProxy(target string) (*InterruptibleProxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &InterruptibleProxy{
		listener: ln,
		target:   target,
		done:     make(chan struct{}),
	}
	go p.acceptLoop()
	return p, nil
}

// Addr returns the proxy's listen address (e.g., "127.0.0.1:54321").
func (p *InterruptibleProxy) Addr() string {
	return p.listener.Addr().String()
}

// DisconnectAll forcibly closes every active proxied connection, simulating
// a network partition from the client's perspective.
func (p *InterruptibleProxy) DisconnectAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.conns {
		_ = c.Close()
	}
	p.conns = nil
}

// Close shuts down the listener and all active connections.
func (p *InterruptibleProxy) Close() {
	close(p.done)
	_ = p.listener.Close()
	p.DisconnectAll()
}

func (p *InterruptibleProxy) acceptLoop() {
	for {
		clientConn, err := p.listener.Accept()
		if err != nil {
			select {
			case <-p.done:
				return
			default:
				continue
			}
		}
		go p.handleConn(clientConn)
	}
}

func (p *InterruptibleProxy) handleConn(clientConn net.Conn) {
	upstreamConn, err := net.Dial("tcp", p.target)
	if err != nil {
		_ = clientConn.Close()
		return
	}

	p.mu.Lock()
	p.conns = append(p.conns, clientConn, upstreamConn)
	p.mu.Unlock()

	go func() { _, _ = io.Copy(upstreamConn, clientConn) }()
	_, _ = io.Copy(clientConn, upstreamConn)
}
