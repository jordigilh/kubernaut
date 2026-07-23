/*
Copyright 2025 Jordi Gil.

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

package datastorage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	config "github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// BR-DS-004: DLQ Fallback HTTP 202 Response During DB Unavailability
//
// Validates the full HTTP path: real request -> ogen router -> handler -> DB error -> DLQ -> HTTP 202.
//
// Parallel-safe: each test creates a dedicated TCP proxy to PostgreSQL. Closing the proxy
// kills all DB connections for ONLY this test's server instance, without affecting other
// parallel tests or the shared PostgreSQL instance.
//
// Coverage:
//   - Unit tier: handler fallback decision logic (mock DB, mock DLQ)
//   - Integration tier (this file): full HTTP stack with real PostgreSQL + real Redis
//   - DLQ client mechanics: test/integration/datastorage/dlq_test.go

// tcpProxy forwards TCP connections to a target address. Closing it terminates
// all forwarded connections, simulating a network partition for the upstream client.
type tcpProxy struct {
	listener net.Listener
	target   string
	mu       sync.Mutex
	conns    []net.Conn
	closed   bool
}

func newTCPProxy(target string) (*tcpProxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &tcpProxy{listener: ln, target: target}
	go p.accept()
	return p, nil
}

func (p *tcpProxy) Addr() string { return p.listener.Addr().String() }

func (p *tcpProxy) accept() {
	for {
		clientConn, err := p.listener.Accept()
		if err != nil {
			return
		}
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			clientConn.Close()
			return
		}
		p.conns = append(p.conns, clientConn)
		p.mu.Unlock()

		go p.forward(clientConn)
	}
}

func (p *tcpProxy) forward(clientConn net.Conn) {
	upstream, err := net.DialTimeout("tcp", p.target, 5*time.Second)
	if err != nil {
		clientConn.Close()
		return
	}
	p.mu.Lock()
	p.conns = append(p.conns, upstream)
	p.mu.Unlock()

	go func() { _, _ = io.Copy(upstream, clientConn) }()
	_, _ = io.Copy(clientConn, upstream)
}

// Disconnect stops the listener and force-closes every forwarded connection,
// making the upstream server's connection pool fail on subsequent queries.
func (p *tcpProxy) Disconnect() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.listener.Close()
	for _, c := range p.conns {
		c.Close()
	}
	p.conns = nil
}

var _ = Describe("BR-DS-004: DLQ Fallback HTTP 202 (Integration)", func() {
	It("IT-DLQ-FALLBACK-001: should return HTTP 202 when PostgreSQL becomes unavailable mid-flight", func() {
		pgHost := os.Getenv("POSTGRES_HOST")
		if pgHost == "" {
			pgHost = localhost
		}
		pgPort := os.Getenv("POSTGRES_PORT")
		if pgPort == "" {
			pgPort = "15433"
		}
		redisHost := os.Getenv("REDIS_HOST")
		if redisHost == "" {
			redisHost = localhost
		}
		redisPort := os.Getenv("REDIS_PORT")
		if redisPort == "" {
			redisPort = "16379"
		}

		By("Starting TCP proxy to PostgreSQL")
		proxy, err := newTCPProxy(net.JoinHostPort(pgHost, pgPort))
		Expect(err).ToNot(HaveOccurred())
		defer proxy.Disconnect()

		proxyHost, proxyPort, err := net.SplitHostPort(proxy.Addr())
		Expect(err).ToNot(HaveOccurred())

		dbConnStr := fmt.Sprintf(
			"host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable",
			proxyHost, proxyPort,
		)
		redisAddr := net.JoinHostPort(redisHost, redisPort)

		By("Creating a dedicated DataStorage server through the proxy")
		appCfg := &config.Config{
			Server: config.ServerConfig{
				SignerCertDir: datastorageIntegrationSigningCertDirOrDie(),
			},
			Database: config.DatabaseConfig{
				MaxOpenConns:    5,
				MaxIdleConns:    2,
				ConnMaxLifetime: "5m",
				ConnMaxIdleTime: "10m",
			},
		}
		serverCfg := &server.Config{
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		mockAuth := &auth.MockAuthenticator{
			ValidUsers: map[string]string{
				"dlq-test-token": "system:serviceaccount:datastorage-test:dlq-fallback-test",
			},
		}
		mockAuthz := &auth.MockAuthorizer{
			AllowedUsers: map[string]bool{
				"system:serviceaccount:datastorage-test:dlq-fallback-test": true,
			},
		}

		srv, err := server.NewServer(server.ServerDeps{
			DBConnStr:     dbConnStr,
			RedisAddr:     redisAddr,
			RedisPassword: "",
			Logger:        logger,
			AppConfig:     appCfg,
			ServerConfig:  serverCfg,
			DLQMaxLen:     1000,
			Authenticator: mockAuth,
			Authorizer:    mockAuthz,
			AuthNamespace: "datastorage-test",
			K8sRestConfig: dsK8sRestConfig,
		})
		Expect(err).ToNot(HaveOccurred(), "Server should start successfully through proxy")
		defer func() { _ = srv.Shutdown(ctx) }()

		ts := httptest.NewServer(srv.Handler())
		defer ts.Close()

		httpClient := &http.Client{
			Transport: &bearerTransport{token: "dlq-test-token"},
			Timeout:   5 * time.Second,
		}

		By("Verifying baseline: POST audit event succeeds with DB available")
		correlationID := fmt.Sprintf("dlq-fallback-baseline-%d-%s", GinkgoParallelProcess(), generateTestUUID())
		baselineBody := buildAuditEventJSON(correlationID, "gateway.signal.received", "baseline_write")
		resp, err := httpClient.Post(ts.URL+"/api/v1/audit/events", "application/json", bytes.NewReader(baselineBody))
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Baseline event should be created (201)")
		resp.Body.Close()

		By("Disconnecting the TCP proxy to simulate DB failure")
		proxy.Disconnect()

		By("Sending audit events until connection pool detects dead connections (HTTP 202)")
		var lastStatus int
		Eventually(func() int {
			cid := fmt.Sprintf("dlq-fallback-outage-%d-%s", GinkgoParallelProcess(), generateTestUUID())
			body := buildAuditEventJSON(cid, "gateway.signal.received", "outage_write")
			resp, err := httpClient.Post(ts.URL+"/api/v1/audit/events", "application/json", bytes.NewReader(body))
			if err != nil {
				return 0
			}
			resp.Body.Close()
			lastStatus = resp.StatusCode
			return resp.StatusCode
		}, "10s", "200ms").Should(Equal(http.StatusAccepted),
			fmt.Sprintf("Expected HTTP 202 (DLQ fallback) after proxy disconnect, last status: %d", lastStatus))

		By("Verifying event was enqueued to Redis DLQ")
		Eventually(func() int64 {
			depth, err := redisClient.XLen(ctx, "audit:dlq:events").Result()
			if err != nil {
				return 0
			}
			return depth
		}, "5s").Should(BeNumerically(">=", 1), "DLQ should contain at least one event")
	})
})

func buildAuditEventJSON(correlationID, eventType, action string) []byte {
	event := map[string]interface{}{
		"version":         "1.0",
		"event_category":  "gateway",
		"event_type":      eventType,
		"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"correlation_id":  correlationID,
		"event_outcome":   "success",
		"event_action":    action,
		"event_data": map[string]interface{}{
			"event_type":  "gateway.signal.received",
			"signal_type": "alert",
			"signal_name": "DLQFallbackTest",
			"namespace":   "default",
			"fingerprint": correlationID,
		},
	}
	data, _ := json.Marshal(event)
	return data
}
