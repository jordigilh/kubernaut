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

// Package redisfault controls only the external Redis wire for lost-reply tests.
package redisfault

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicebob/miniredis/v2/proto"
)

// Proxy forwards RESP requests to a real or controlled Redis server. It can
// discard one successful EVALSHA response *after* receiving the entire reply,
// proving execution before disconnect, while continuing to accept reconnects.
type Proxy struct {
	listener net.Listener
	backend  string
	mu       sync.Mutex
	armed    string
	calls    map[string]int
	conns    map[net.Conn]struct{}
	closed   bool
	wg       sync.WaitGroup
}

func New(ctx context.Context, backend string) (*Proxy, error) {
	var config net.ListenConfig
	listener, err := config.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for redis fault proxy: %w", err)
	}
	p := &Proxy{listener: listener, backend: backend, calls: make(map[string]int), conns: make(map[net.Conn]struct{})}
	p.wg.Add(1)
	go p.accept(ctx)
	return p, nil
}

func (p *Proxy) Addr() string { return p.listener.Addr().String() }

func (p *Proxy) Arm(verb string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.armed = verb
}

func (p *Proxy) Calls(verb string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[verb]
}

func (p *Proxy) Close() {
	p.mu.Lock()
	p.closed = true
	_ = p.listener.Close()
	for conn := range p.conns {
		_ = conn.Close()
	}
	p.mu.Unlock()
	p.wg.Wait()
}

func (p *Proxy) accept(ctx context.Context) {
	defer p.wg.Done()
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return
		}
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			_ = conn.Close()
			return
		}
		p.conns[conn] = struct{}{}
		p.wg.Add(1)
		p.mu.Unlock()
		go p.forward(ctx, conn)
	}
}

func (p *Proxy) forward(ctx context.Context, client net.Conn) {
	defer p.wg.Done()
	defer func() {
		_ = client.Close()
		p.mu.Lock()
		delete(p.conns, client)
		p.mu.Unlock()
	}()
	dialer := net.Dialer{Timeout: 5 * time.Second}
	backend, err := dialer.DialContext(ctx, "tcp", p.backend)
	if err != nil {
		return
	}
	defer func() { _ = backend.Close() }()
	requests, replies := bufio.NewReader(client), bufio.NewReader(backend)
	for {
		request, err := proto.Read(requests)
		if err != nil {
			return
		}
		verb := scriptVerb(request)
		p.mu.Lock()
		p.calls[verb]++
		p.mu.Unlock()
		if err := backend.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return
		}
		if _, err := io.WriteString(backend, request); err != nil {
			return
		}
		reply, err := proto.Read(replies)
		if err != nil {
			return
		}
		p.mu.Lock()
		drop := verb != "" && p.armed == verb && !strings.HasPrefix(reply, "-")
		if drop {
			p.armed = ""
		}
		p.mu.Unlock()
		if drop {
			return
		}
		if _, err := io.WriteString(client, reply); err != nil {
			return
		}
	}
}

func scriptVerb(request string) string {
	args, err := proto.ReadStrings(request)
	if err != nil || len(args) < 4 || !strings.EqualFold(args[0], "evalsha") {
		return ""
	}
	keys, err := strconv.Atoi(args[2])
	if err != nil || keys < 0 || 3+keys >= len(args) {
		return ""
	}
	return args[3+keys]
}
