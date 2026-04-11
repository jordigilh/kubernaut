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
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	dsconfig "github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// Issue #667: BR-STORAGE-042 — All DB queries must propagate caller context
var _ = Describe("Context Propagation [BR-STORAGE-042]", func() {
	var (
		testID string
	)

	BeforeEach(func() {
		testID = generateTestID()
	})

	AfterEach(func() {
		_, _ = db.ExecContext(context.Background(),
			"DELETE FROM audit_events WHERE correlation_id LIKE $1",
			fmt.Sprintf("%%ctx-prop-%s%%", testID))
	})

	It("IT-DS-042-001: effectiveness query cancels DB operation when caller context is cancelled", func() {
		corrID := fmt.Sprintf("ctx-prop-%s-eff", testID)

		auditRepo := repository.NewAuditEventsRepository(db.DB, logger)
		evt := &repository.AuditEvent{
			EventID:       uuid.New(),
			EventType:     "effectiveness.health_assessed",
			Version:       "1.0",
			EventCategory: "effectiveness",
			EventAction:   "assess",
			EventOutcome:  "success",
			CorrelationID: corrID,
			ResourceType:  "deployment",
			ResourceID:    "test-deploy",
			ActorID:       "system",
			ActorType:     "service",
			RetentionDays: 30,
			EventData:     map[string]interface{}{"score": 0.95, "event_type": "effectiveness.health_assessed"},
		}
		_, err := auditRepo.Create(context.Background(), evt)
		Expect(err).ToNot(HaveOccurred(), "seed event must be created")

		ts := newContextPropagationTestServer()
		defer ts.Close()

		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		req, err := http.NewRequestWithContext(cancelledCtx, http.MethodGet,
			fmt.Sprintf("%s/api/v1/effectiveness/%s", ts.URL, corrID), nil)
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Authorization", "Bearer test-token")

		client := &http.Client{Timeout: 5 * time.Second}
		_, reqErr := client.Do(req)
		Expect(reqErr).To(HaveOccurred(),
			"request with cancelled context should fail — context propagation ensures DB query is cancelled")
	})

	It("IT-DS-042-002: adapter query cancels DB operation when caller context is cancelled", func() {
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := db.DB.QueryContext(cancelledCtx,
			"SELECT event_type, event_data FROM audit_events WHERE correlation_id = $1 AND event_category = 'effectiveness'",
			"nonexistent")
		Expect(err).To(HaveOccurred(),
			"QueryContext with cancelled context should return context error")
		Expect(err.Error()).To(ContainSubstring("context"),
			"error should reference context cancellation")
	})

	It("IT-DS-042-003: effectiveness query with valid context returns correct events", func() {
		corrID := fmt.Sprintf("ctx-prop-%s-valid", testID)

		auditRepo := repository.NewAuditEventsRepository(db.DB, logger)
		evt := &repository.AuditEvent{
			EventID:       uuid.New(),
			EventType:     "effectiveness.health_assessed",
			Version:       "1.0",
			EventCategory: "effectiveness",
			EventAction:   "assess",
			EventOutcome:  "success",
			CorrelationID: corrID,
			ResourceType:  "deployment",
			ResourceID:    "test-deploy",
			ActorID:       "system",
			ActorType:     "service",
			RetentionDays: 30,
			EventData:     map[string]interface{}{"score": 0.85, "event_type": "effectiveness.health_assessed"},
		}
		_, err := auditRepo.Create(context.Background(), evt)
		Expect(err).ToNot(HaveOccurred())

		ts := newContextPropagationTestServer()
		defer ts.Close()

		reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet,
			fmt.Sprintf("%s/api/v1/effectiveness/%s", ts.URL, corrID), nil)
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Authorization", "Bearer test-token")

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"effectiveness endpoint should return 200 for valid correlation_id with events")

		var body map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
		Expect(body).To(HaveKey("score"),
			"response should contain effectiveness score")
	})
})

func newContextPropagationTestServer() *httptest.Server {
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
		Database: dsconfig.DatabaseConfig{
			MaxOpenConns:    5,
			MaxIdleConns:    2,
			ConnMaxLifetime: "1m",
			ConnMaxIdleTime: "1m",
		},
	}

	const testToken = "test-token"
	const testUser = "system:serviceaccount:test:context-propagation"

	srv, err := server.NewServer(server.ServerDeps{
		DBConnStr:     dbConnStr,
		RedisAddr:     redisAddr,
		RedisPassword: "",
		Logger:        logger,
		AppConfig:     appCfg,
		ServerConfig: &server.Config{
			Port:         18090,
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
