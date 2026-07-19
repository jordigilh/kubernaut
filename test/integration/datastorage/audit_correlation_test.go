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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	dsconfig "github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/query"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// Issue #1199: Bidirectional correlation query integration tests.
// These tests validate the full write→query round trip against real PostgreSQL
// using the event_data JSONB ->> filter (detail_key / detail_value).
//
// BR-STORAGE-022: Query Filtering
// BR-STORAGE-032: Unified audit trail for cross-service correlation

var _ = Describe("Audit Correlation Query Integration Tests (Issue #1199)", func() {

	insertCorrelationEvent := func(correlationID, eventType, taskID, rrName, rrNamespace string) {
		eventID := uuid.New().String()
		now := time.Now().UTC()
		eventDate := now.Truncate(24 * time.Hour)

		eventData := map[string]interface{}{
			"event_type": eventType,
			"session_id": "sess-correlation-test",
			"task_id":    taskID,
		}
		if rrName != "" {
			eventData["rr_name"] = rrName
		}
		if rrNamespace != "" {
			eventData["rr_namespace"] = rrNamespace
		}

		eventDataJSON, err := json.Marshal(eventData)
		Expect(err).ToNot(HaveOccurred())

		_, err = db.Exec(`
			INSERT INTO audit_events (
				event_id, event_timestamp, event_date, event_type, event_category,
				correlation_id, resource_type, resource_id, event_action,
				event_outcome, actor_type, actor_id, event_data
			) VALUES (
				$1, $2, $3, $4, 'apifrontend', $5,
				'a2a_task', $6, 'completed', 'success', 'service', 'apifrontend',
				$7::jsonb
			)
		`, eventID, now, eventDate, eventType, correlationID, taskID, string(eventDataJSON))
		Expect(err).ToNot(HaveOccurred(), "inserting correlation test event should succeed")
	}

	// ========================================
	// TIER 2: Query Builder → Real PostgreSQL
	// ========================================

	Context("Query builder against real PostgreSQL", func() {
		var (
			auditRepo *repository.AuditEventsRepository
			testID    string
		)

		BeforeEach(func() {
			auditRepo = repository.NewAuditEventsRepository(db.DB, logger)
			testID = fmt.Sprintf("corr-1199-%s", uuid.New().String()[:8])
		})

		AfterEach(func() {
			if db != nil {
				_, _ = db.ExecContext(ctx,
					"DELETE FROM audit_events WHERE correlation_id LIKE $1",
					fmt.Sprintf("%s%%", testID))
			}
		})

		It("IT-DS-1199-010: query by detail_key=task_id returns event with rr_name", func() {
			taskID := fmt.Sprintf("task-%s", uuid.New().String()[:8])
			insertCorrelationEvent(testID, "apifrontend.a2a.task_completed", taskID, "rr-oom-web", "production")

			builder := query.NewAuditEventsQueryBuilder(query.WithAuditEventsLogger(logger)).
				WithEventDataFilter("task_id", taskID)

			querySQL, queryArgs, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			countSQL, _, err := builder.BuildCount()
			Expect(err).ToNot(HaveOccurred())

			events, pagination, err := auditRepo.Query(ctx, querySQL, countSQL, queryArgs)
			Expect(err).ToNot(HaveOccurred())
			Expect(pagination.Total).To(BeNumerically(">=", 1))
			Expect(events).To(HaveLen(1))

			Expect(events[0].EventData).To(HaveKeyWithValue("rr_name", "rr-oom-web"))
			Expect(events[0].EventData).To(HaveKeyWithValue("task_id", taskID))
		})

		It("IT-DS-1199-011: query by detail_key=rr_name returns event with task_id (reverse direction)", func() {
			taskID := fmt.Sprintf("task-%s", uuid.New().String()[:8])
			rrName := fmt.Sprintf("rr-%s", uuid.New().String()[:8])
			insertCorrelationEvent(testID, "apifrontend.a2a.task_completed", taskID, rrName, "staging")

			builder := query.NewAuditEventsQueryBuilder(query.WithAuditEventsLogger(logger)).
				WithEventDataFilter("rr_name", rrName)

			querySQL, queryArgs, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			countSQL, _, err := builder.BuildCount()
			Expect(err).ToNot(HaveOccurred())

			events, pagination, err := auditRepo.Query(ctx, querySQL, countSQL, queryArgs)
			Expect(err).ToNot(HaveOccurred())
			Expect(pagination.Total).To(BeNumerically(">=", 1))
			Expect(events).To(HaveLen(1))

			Expect(events[0].EventData).To(HaveKeyWithValue("task_id", taskID))
			Expect(events[0].EventData).To(HaveKeyWithValue("rr_name", rrName))
		})

		It("IT-DS-1199-013: query with non-existent detail_value returns empty result", func() {
			builder := query.NewAuditEventsQueryBuilder(query.WithAuditEventsLogger(logger)).
				WithEventDataFilter("task_id", "nonexistent-task-id-"+uuid.New().String())

			querySQL, queryArgs, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			countSQL, _, err := builder.BuildCount()
			Expect(err).ToNot(HaveOccurred())

			events, pagination, err := auditRepo.Query(ctx, querySQL, countSQL, queryArgs)
			Expect(err).ToNot(HaveOccurred())
			Expect(events).To(BeEmpty())
			Expect(pagination.Total).To(Equal(0))
		})

		It("IT-DS-1199-014: query combining detail_key + event_type returns only matching events", func() {
			taskID := fmt.Sprintf("task-%s", uuid.New().String()[:8])
			insertCorrelationEvent(testID, "apifrontend.a2a.task_completed", taskID, "rr-web", "default")
			insertCorrelationEvent(testID, "apifrontend.a2a.task_failed", taskID, "rr-web", "default")

			builder := query.NewAuditEventsQueryBuilder(query.WithAuditEventsLogger(logger)).
				WithEventType("apifrontend.a2a.task_completed").
				WithEventDataFilter("task_id", taskID)

			querySQL, queryArgs, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			countSQL, _, err := builder.BuildCount()
			Expect(err).ToNot(HaveOccurred())

			events, pagination, err := auditRepo.Query(ctx, querySQL, countSQL, queryArgs)
			Expect(err).ToNot(HaveOccurred())
			Expect(pagination.Total).To(Equal(1))
			Expect(events).To(HaveLen(1))
			Expect(events[0].EventType).To(Equal("apifrontend.a2a.task_completed"))
		})
	})

	// ========================================
	// TIER 2: HTTP API Validation Paths
	// ========================================

	Context("HTTP API validation (in-process DS server)", func() {
		var ts *httptest.Server

		BeforeEach(func() {
			pgHost := os.Getenv("POSTGRES_HOST")
			if pgHost == "" {
				pgHost = "localhost"
			}
			pgPort := os.Getenv("POSTGRES_PORT")
			if pgPort == "" {
				pgPort = "15433"
			}
			dbConnStr := fmt.Sprintf(
				"host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable options='-c search_path=public'",
				pgHost, pgPort,
			)

			redisHost := os.Getenv("REDIS_HOST")
			if redisHost == "" {
				redisHost = "localhost"
			}
			redisPort := os.Getenv("REDIS_PORT")
			if redisPort == "" {
				redisPort = "16379"
			}
			redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort)

			const testToken = "it-1199-token"
			const testUser = "system:serviceaccount:datastorage-test:it-1199"

			appCfg := &dsconfig.Config{
				Server: dsconfig.ServerConfig{
					SignerCertDir: datastorageIntegrationSigningCertDirOrDie(),
				},
				Database: dsconfig.DatabaseConfig{
					MaxOpenConns:    5,
					MaxIdleConns:    2,
					ConnMaxLifetime: "1m",
					ConnMaxIdleTime: "1m",
				},
			}

			srv, err := server.NewServer(server.ServerDeps{
				DBConnStr:     dbConnStr,
				RedisAddr:     redisAddr,
				RedisPassword: "",
				Logger:        logr.Discard(),
				AppConfig:     appCfg,
				ServerConfig: &server.Config{
					Port:         18091,
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
				AuthNamespace: "datastorage-test",
				K8sRestConfig: dsK8sRestConfig,
			})
			Expect(err).ToNot(HaveOccurred())

			ts = httptest.NewServer(srv.Handler())
		})

		AfterEach(func() {
			if ts != nil {
				ts.Close()
			}
		})

		authedGet := func(url string) (*http.Response, error) {
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer it-1199-token")
			return http.DefaultClient.Do(req)
		}

		It("IT-DS-1199-015: detail_key without detail_value returns 400", func() {
			resp, err := authedGet(ts.URL + "/api/v1/audit/events?detail_key=task_id")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})

		It("IT-DS-1199-016: detail_value without detail_key returns 400", func() {
			resp, err := authedGet(ts.URL + "/api/v1/audit/events?detail_value=task-abc")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})

		It("IT-DS-1199-017: invalid detail_key with special characters returns 400", func() {
			resp, err := authedGet(ts.URL + "/api/v1/audit/events?detail_key=task_id%27%3B+DROP+TABLE--&detail_value=val")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})
})
