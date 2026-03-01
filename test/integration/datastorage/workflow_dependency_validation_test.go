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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// ========================================
// DD-WE-006: Schema-Declared Dependency Validation (Registration-Time)
// ========================================
// Authority: DD-WE-006, BR-WORKFLOW-004
// Tests: Dependency validation during workflow registration via DS HTTP API
//
// Pattern: In-process httptest.Server with MockImagePuller (controlled schema)
// + envtest K8s client (real K8s API) + real PostgreSQL/Redis
// Per TESTING_GUIDELINES.md: Integration tests use envtest for K8s
// ========================================

const (
	depTestNamespace = "kubernaut-workflows"

	// Base schema without dependencies for backward compat tests
	depTestBaseSchema = `metadata:
  workflowId: dep-test-workflow
  version: "1.0.0"
  description:
    what: Integration test workflow for dependency validation
    whenToUse: When testing DD-WE-006
  labels:
    severity: [critical]
    environment: ["*"]
    component: deployment
    priority: "*"
actionType: GitRevertCommit
execution:
  engine: job
  bundle: quay.io/kubernaut-cicd/test-workflows/dep-test:v1.0.0
parameters:
  - name: TARGET_NAMESPACE
    type: string
    required: true
    description: Namespace of the affected resource
`
)

func depTestSchemaWithSecrets(secretNames ...string) string {
	if len(secretNames) == 0 {
		return depTestBaseSchema
	}
	deps := "dependencies:\n  secrets:\n"
	for _, name := range secretNames {
		deps += fmt.Sprintf("    - name: %s\n", name)
	}
	return depTestBaseSchema + deps
}

func depTestSchemaWithConfigMaps(cmNames ...string) string {
	if len(cmNames) == 0 {
		return depTestBaseSchema
	}
	deps := "dependencies:\n  configMaps:\n"
	for _, name := range cmNames {
		deps += fmt.Sprintf("    - name: %s\n", name)
	}
	return depTestBaseSchema + deps
}

// createDepTestServer creates an in-process DS server with dependency validation enabled.
// Uses the suite's envtest k8sClient for real K8s API validation of Secrets/ConfigMaps.
func createDepTestServer(schemaYAML string) (*httptest.Server, *server.Server) {
	serverCfg := &server.Config{
		Port:         18090,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	pgHost := os.Getenv("POSTGRES_HOST")
	if pgHost == "" {
		pgHost = "localhost"
	}
	pgPort := os.Getenv("POSTGRES_PORT")
	if pgPort == "" {
		pgPort = "5433"
	}
	dbConnStr := fmt.Sprintf("host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable", pgHost, pgPort)

	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}
	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = "6379"
	}
	redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort)

	appCfg := &config.Config{
		Database: config.DatabaseConfig{
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: "5m",
			ConnMaxIdleTime: "10m",
		},
	}

	mockAuthenticator := &auth.MockAuthenticator{
		ValidUsers: map[string]string{
			"test-token": "system:serviceaccount:datastorage-test:dep-validation-test",
		},
	}
	mockAuthorizer := &auth.MockAuthorizer{
		AllowedUsers: map[string]bool{
			"system:serviceaccount:datastorage-test:dep-validation-test": true,
		},
	}

	mockPuller := oci.NewMockImagePuller(schemaYAML)
	schemaExtractor := oci.NewSchemaExtractor(mockPuller, schema.NewParser())
	depValidator := validation.NewK8sDependencyValidator(k8sClient)

	srv, err := server.NewServer(server.ServerDeps{
		DBConnStr:     dbConnStr,
		RedisAddr:     redisAddr,
		RedisPassword: "",
		Logger:        logger,
		AppConfig:     appCfg,
		ServerConfig:  serverCfg,
		DLQMaxLen:     1000,
		Authenticator: mockAuthenticator,
		Authorizer:    mockAuthorizer,
		AuthNamespace: "datastorage-test",
		HandlerOpts: []server.HandlerOption{
			server.WithSchemaExtractor(schemaExtractor),
			server.WithDependencyValidator(depValidator, depTestNamespace),
		},
	})
	Expect(err).ToNot(HaveOccurred(), "Server creation should succeed")

	httpServer := httptest.NewServer(srv.Handler())
	return httpServer, srv
}

// deleteK8sObject removes a K8s object, ignoring not-found errors.
func deleteK8sObject(obj client.Object) {
	_ = k8sClient.Delete(ctx, obj)
}

func registerWorkflow(serverURL string) (*http.Response, error) {
	body := `{"schemaImage":"test-registry.io/dep-test:v1.0.0"}`
	req, err := http.NewRequest("POST", serverURL+"/api/v1/workflows", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

var _ = Describe("Schema-Declared Dependency Validation (DD-WE-006)", Label("integration", "dd-we-006"), func() {

	Context("Registration-time K8s validation", func() {

		It("IT-DS-006-001: should accept workflow when all declared secrets exist with data", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "it-gitea-creds-001", Namespace: depTestNamespace},
				Data:       map[string][]byte{"username": []byte("kubernaut"), "password": []byte("s3cret")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer deleteK8sObject(secret)

			schemaYAML := depTestSchemaWithSecrets("it-gitea-creds-001")
			testServer, srv := createDepTestServer(schemaYAML)
			defer testServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			resp, err := registerWorkflow(testServer.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"registration should succeed when declared secret exists with data")
		})

		It("IT-DS-006-002: should reject workflow when declared Secret is missing", func() {
			schemaYAML := depTestSchemaWithSecrets("it-missing-secret-002")
			testServer, srv := createDepTestServer(schemaYAML)
			defer testServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			resp, err := registerWorkflow(testServer.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"registration should fail when declared secret does not exist")

			body, _ := io.ReadAll(resp.Body)
			var errResp map[string]interface{}
			Expect(json.Unmarshal(body, &errResp)).To(Succeed())
			detail, _ := errResp["detail"].(string)
			Expect(detail).To(ContainSubstring("it-missing-secret-002"),
				"error should name the missing resource")
		})

		It("IT-DS-006-003: should reject workflow when declared Secret has empty data", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "it-empty-secret-003", Namespace: depTestNamespace},
				Data:       map[string][]byte{},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer deleteK8sObject(secret)

			schemaYAML := depTestSchemaWithSecrets("it-empty-secret-003")
			testServer, srv := createDepTestServer(schemaYAML)
			defer testServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			resp, err := registerWorkflow(testServer.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"registration should fail when declared secret has empty data")

			body, _ := io.ReadAll(resp.Body)
			var errResp map[string]interface{}
			Expect(json.Unmarshal(body, &errResp)).To(Succeed())
			detail, _ := errResp["detail"].(string)
			Expect(detail).To(ContainSubstring("empty"),
				"error should indicate empty data")
		})

		It("IT-DS-006-004: should reject workflow when declared ConfigMap is missing", func() {
			schemaYAML := depTestSchemaWithConfigMaps("it-missing-cm-004")
			testServer, srv := createDepTestServer(schemaYAML)
			defer testServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			resp, err := registerWorkflow(testServer.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"registration should fail when declared configMap does not exist")

			body, _ := io.ReadAll(resp.Body)
			var errResp map[string]interface{}
			Expect(json.Unmarshal(body, &errResp)).To(Succeed())
			detail, _ := errResp["detail"].(string)
			Expect(detail).To(ContainSubstring("it-missing-cm-004"),
				"error should name the missing resource")
		})

		It("IT-DS-006-005: should reject workflow when declared ConfigMap has empty data", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "it-empty-cm-005", Namespace: depTestNamespace},
				Data:       map[string]string{},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())
			defer deleteK8sObject(cm)

			schemaYAML := depTestSchemaWithConfigMaps("it-empty-cm-005")
			testServer, srv := createDepTestServer(schemaYAML)
			defer testServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			resp, err := registerWorkflow(testServer.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"registration should fail when declared configMap has empty data")

			body, _ := io.ReadAll(resp.Body)
			var errResp map[string]interface{}
			Expect(json.Unmarshal(body, &errResp)).To(Succeed())
			detail, _ := errResp["detail"].(string)
			Expect(detail).To(ContainSubstring("empty"),
				"error should indicate empty data")
		})

		It("IT-DS-006-006: should accept workflow without dependencies section", func() {
			testServer, srv := createDepTestServer(depTestBaseSchema)
			defer testServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			resp, err := registerWorkflow(testServer.URL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"registration should succeed for workflows without dependencies")
		})
	})
})
