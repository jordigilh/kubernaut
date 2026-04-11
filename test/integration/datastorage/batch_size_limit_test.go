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

package datastorage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	dsconfig "github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// Issue #667: BR-STORAGE-043 — HTTP batch API must enforce a maximum batch size
var _ = Describe("Batch Size Limit [BR-STORAGE-043]", func() {
	var (
		testID string
	)

	BeforeEach(func() {
		testID = generateTestID()
	})

	AfterEach(func() {
		_, _ = db.ExecContext(context.Background(),
			"DELETE FROM audit_events WHERE correlation_id LIKE $1",
			fmt.Sprintf("%%batch-limit-%s%%", testID))
	})

	It("IT-DS-043-001: batch API returns 400 RFC7807 when payload exceeds configured maximum", func() {
		ts := newBatchLimitTestServer(5)
		defer ts.Close()

		events := buildAuditEventBatch(testID, 6)
		body, err := json.Marshal(events)
		Expect(err).ToNot(HaveOccurred())

		req, err := http.NewRequest(http.MethodPost,
			ts.URL+"/api/v1/audit/events/batch",
			strings.NewReader(string(body)))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
			"batch exceeding MaxBatchSize must be rejected with 400")

		var rfc7807 map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&rfc7807)).To(Succeed())
		Expect(rfc7807).To(HaveKey("type"),
			"error response must be RFC7807 format with 'type' field")
		detail, ok := rfc7807["detail"].(string)
		Expect(ok).To(BeTrue())
		Expect(detail).To(ContainSubstring("batch size"),
			"error detail should reference the batch size limit")
	})

	It("IT-DS-043-002: batch API accepts and persists payload at exactly the configured maximum", func() {
		ts := newBatchLimitTestServer(5)
		defer ts.Close()

		events := buildAuditEventBatch(testID, 5)
		body, err := json.Marshal(events)
		Expect(err).ToNot(HaveOccurred())

		req, err := http.NewRequest(http.MethodPost,
			ts.URL+"/api/v1/audit/events/batch",
			strings.NewReader(string(body)))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusCreated),
			"batch at exactly MaxBatchSize must be accepted")
	})

	It("IT-DS-043-003: batch API with zero MaxBatchSize in config uses default (500) and enforces it", func() {
		ts := newBatchLimitTestServer(0)
		defer ts.Close()

		events := buildAuditEventBatch(testID, 501)
		body, err := json.Marshal(events)
		Expect(err).ToNot(HaveOccurred())

		req, err := http.NewRequest(http.MethodPost,
			ts.URL+"/api/v1/audit/events/batch",
			strings.NewReader(string(body)))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
			"default MaxBatchSize of 500 must be enforced when config is 0")
	})
})

func newBatchLimitTestServer(maxBatchSize int) *httptest.Server {
	pgHost := os.Getenv("POSTGRES_HOST")
	if pgHost == "" {
		pgHost = "localhost"
	}
	pgPort := os.Getenv("POSTGRES_PORT")
	if pgPort == "" {
		pgPort = "15433"
	}
	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}
	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = "16379"
	}

	dbConnStr := fmt.Sprintf(
		"host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable options='-c search_path=public'",
		pgHost, pgPort,
	)
	redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort)

	appCfg := &dsconfig.Config{
		Server: dsconfig.ServerConfig{
			MaxBatchSize: maxBatchSize,
		},
		Database: dsconfig.DatabaseConfig{
			MaxOpenConns:    5,
			MaxIdleConns:    2,
			ConnMaxLifetime: "1m",
			ConnMaxIdleTime: "1m",
		},
	}

	const testToken = "test-token"
	const testUser = "system:serviceaccount:test:batch-limit"

	srv, err := server.NewServer(server.ServerDeps{
		DBConnStr:     dbConnStr,
		RedisAddr:     redisAddr,
		RedisPassword: "",
		Logger:        logger,
		AppConfig:     appCfg,
		ServerConfig: &server.Config{
			Port:         18092,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		DLQMaxLen: 100,
		Authenticator: &auth.MockAuthenticator{
			ValidUsers: map[string]string{
				testToken: testUser,
			},
		},
		Authorizer: &auth.MockAuthorizer{
			AllowedUsers: map[string]bool{
				testUser: true,
			},
		},
		AuthNamespace: "test",
	})
	Expect(err).ToNot(HaveOccurred())

	return httptest.NewServer(srv.Handler())
}

func buildAuditEventBatch(testID string, count int) []map[string]interface{} {
	events := make([]map[string]interface{}, count)
	for i := 0; i < count; i++ {
		events[i] = map[string]interface{}{
			"event_id":       uuid.New().String(),
			"event_type":     "gateway.signal.received",
			"event_version":  "1.0",
			"event_category": "gateway",
			"event_action":   "received",
			"event_outcome":  "success",
			"correlation_id": fmt.Sprintf("batch-limit-%s-%d", testID, i),
			"resource_type":  "Signal",
			"resource_id":    fmt.Sprintf("fp-%d", i),
			"actor_id":       "gateway-service",
			"actor_type":     "service",
			"retention_days": 30,
			"event_data": map[string]interface{}{
				"event_type":  "gateway.signal.received",
				"signal_name": fmt.Sprintf("test-signal-%d", i),
			},
		}
	}
	return events
}
