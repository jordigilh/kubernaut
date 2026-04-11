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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	dsconfig "github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	deterministicuuid "github.com/jordigilh/kubernaut/pkg/datastorage/uuid"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	"github.com/jordigilh/kubernaut/test/testutil"
)

var _ = Describe("Deterministic UUID Integration (#548)", Ordered, func() {

	var (
		ts     *httptest.Server
		client *http.Client
	)

	const testToken = "det-uuid-test-token"
	const testUser = "system:serviceaccount:test:det-uuid-tester"

	BeforeAll(func() {
		host := getEnvOrDefault("POSTGRES_HOST", "localhost")
		pgPort := getEnvOrDefault("POSTGRES_PORT", "15433")
		dbConnStr := fmt.Sprintf(
			"host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable options='-c search_path=public'",
			host, pgPort,
		)
		redisHost := getEnvOrDefault("REDIS_HOST", "localhost")
		redisPort := getEnvOrDefault("REDIS_PORT", "16379")
		redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort)

		appCfg := &dsconfig.Config{
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
				Port:         18090,
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
			},
			DLQMaxLen: 100,
			Authenticator: &auth.MockAuthenticator{
				ValidUsers: map[string]string{testToken: testUser},
			},
			Authorizer: &auth.MockAuthorizer{
				AllowedUsers: map[string]bool{testUser: true},
			},
			AuthNamespace: "test",
			HandlerOpts: []server.HandlerOption{
				server.WithSchemaExtractor(nil),
			},
		})
		Expect(err).ToNot(HaveOccurred())

		ts = httptest.NewServer(srv.Handler())
		client = &http.Client{
			Transport: &bearerTransport{token: testToken},
		}

		By("Seeding ScaleMemory action type (not in suite's standard seed set)")
		_, err = db.ExecContext(context.Background(),
			`INSERT INTO action_type_taxonomy (action_type, description, status)
			 VALUES ($1, '{"what":"test action type","whenToUse":"integration tests"}'::jsonb, 'Active')
			 ON CONFLICT (action_type) DO NOTHING`, "ScaleMemory")
		Expect(err).ToNot(HaveOccurred(), "seeding ScaleMemory should succeed")
	})

	AfterAll(func() {
		if ts != nil {
			ts.Close()
		}
		_, _ = db.ExecContext(context.Background(),
			"DELETE FROM action_type_taxonomy WHERE action_type = $1", "ScaleMemory")
	})

	// ========================================
	// IT-DS-548-001: Full-stack deterministic UUID round-trip
	// ========================================
	Describe("IT-DS-548-001: Full-stack deterministic UUID round-trip", func() {
		It("should assign a deterministic UUID derived from content hash through the full API stack", func() {
			crd := testutil.NewTestWorkflowCRD("det-uuid-it-001", "ScaleMemory", "job")
			content := testutil.MarshalWorkflowCRD(crd)

			body := map[string]string{"content": content}
			jsonBody, err := json.Marshal(body)
			Expect(err).ToNot(HaveOccurred())

			resp, err := client.Post(ts.URL+"/api/v1/workflows", "application/json", bytes.NewReader(jsonBody))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			GinkgoWriter.Printf("IT-DS-548-001: status=%d body=%s\n", resp.StatusCode, string(respBody))

			Expect(resp.StatusCode).To(Or(Equal(http.StatusCreated), Equal(http.StatusOK)),
				"workflow registration should succeed")

			var result map[string]interface{}
			Expect(json.Unmarshal(respBody, &result)).To(Succeed())

			returnedUUID, ok := result["workflowId"].(string)
			Expect(ok).To(BeTrue(), "response should contain workflowId")
			Expect(returnedUUID).ToNot(BeEmpty(), "workflowId should not be empty")

			contentHash, ok := result["contentHash"].(string)
			if ok && contentHash != "" {
				expectedUUID := deterministicuuid.DeterministicUUID(contentHash)
				Expect(returnedUUID).To(Equal(expectedUUID),
					"returned UUID should equal DeterministicUUID(contentHash)")
			}
		})
	})

	// ========================================
	// IT-DS-548-002: PVC-wipe simulation — same content yields same UUID
	// ========================================
	Describe("IT-DS-548-002: PVC-wipe simulation — re-register yields same UUID", func() {
		It("should return the same deterministic UUID when re-registering identical content", func() {
			crd := testutil.NewTestWorkflowCRD("det-uuid-it-002", "RestartPod", "job")
			content := testutil.MarshalWorkflowCRD(crd)

			body := map[string]string{"content": content}
			jsonBody, err := json.Marshal(body)
			Expect(err).ToNot(HaveOccurred())

			// First registration
			resp1, err := client.Post(ts.URL+"/api/v1/workflows", "application/json", bytes.NewReader(jsonBody))
			Expect(err).ToNot(HaveOccurred())
			defer resp1.Body.Close()

			resp1Body, err := io.ReadAll(resp1.Body)
			Expect(err).ToNot(HaveOccurred())
			GinkgoWriter.Printf("IT-DS-548-002 (1st): status=%d body=%s\n", resp1.StatusCode, string(resp1Body))

			Expect(resp1.StatusCode).To(Or(Equal(http.StatusCreated), Equal(http.StatusOK)),
				"first registration should succeed")
			var result1 map[string]interface{}
			Expect(json.Unmarshal(resp1Body, &result1)).To(Succeed())
			firstUUID := result1["workflowId"].(string)
			Expect(len(firstUUID)).To(Equal(36), "workflowId should be a 36-char UUID string")

			// Simulate PVC-wipe: disable the workflow, then delete the DB row
			if firstUUID != "" {
				_, _ = db.ExecContext(context.Background(),
					"DELETE FROM remediation_workflow_catalog WHERE workflow_id = $1", firstUUID)
			}

			// Re-registration with identical content
			jsonBody2, _ := json.Marshal(body)
			resp2, err := client.Post(ts.URL+"/api/v1/workflows", "application/json", bytes.NewReader(jsonBody2))
			Expect(err).ToNot(HaveOccurred())
			defer resp2.Body.Close()

			resp2Body, err := io.ReadAll(resp2.Body)
			Expect(err).ToNot(HaveOccurred())
			GinkgoWriter.Printf("IT-DS-548-002 (2nd): status=%d body=%s\n", resp2.StatusCode, string(resp2Body))

			Expect(resp2.StatusCode).To(Or(Equal(http.StatusCreated), Equal(http.StatusOK)),
				"re-registration should succeed")
			var result2 map[string]interface{}
			Expect(json.Unmarshal(resp2Body, &result2)).To(Succeed())
			secondUUID := result2["workflowId"].(string)

			Expect(secondUUID).To(Equal(firstUUID),
				"PVC-wipe re-registration should recover the same deterministic UUID")
		})
	})
})

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
