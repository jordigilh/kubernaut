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
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	quayIoKubernautCicdTest = "quay.io/kubernaut-cicd/test-workflows/dep-test:v1.0.0@sha256:f313b9632f3a8d0ffd41150b12715a43a41c6c8e7871bb830fd82c09b5988cc4"
)

// ========================================
// Issue #1481: Registration No Longer Pre-Flight-Validates Dependencies
// ========================================
// Authority: BR-PLATFORM-054, Issue #1481
// Supersedes: DD-WE-006 registration-time (Level 2) dependency validation,
// which required Secrets/ConfigMaps to exist (with non-empty data) in the
// execution namespace *before* a workflow could be registered. Issue #1481
// removed the K8sDependencyValidator pre-flight check entirely: dependency
// existence is now validated exclusively at runtime by Kubernetes when the
// WorkflowExecution's Job/PipelineRun attempts to mount the volume/workspace
// (BR-WORKFLOW-008 covers the resulting fail-fast/observability guarantees).
//
// Pattern: In-process httptest.Server with MockImagePuller (controlled schema)
// + real PostgreSQL/Redis. No K8s client is needed any more for these tests
// since dependency existence is never checked at registration time.
// ========================================

const depTestNamespace = "kubernaut-workflows"

func depTestBaseSchemaUnique() string {
	uniqueID := fmt.Sprintf("dep-test-workflow-%d-%s", GinkgoParallelProcess(), uuid.New().String())
	crd := testutil.NewTestWorkflowCRD(uniqueID, "RestartPod", "job")
	crd.Spec.Description = sharedtypes.StructuredDescription{
		What:      "Integration test workflow for dependency validation removal (#1481)",
		WhenToUse: "When testing Issue #1481",
	}
	crd.Spec.Labels.Severity = []string{"critical"}
	crd.Spec.Labels.Environment = []string{"*"}
	crd.Spec.Labels.Component = []string{"apps/v1/Deployment"}
	crd.Spec.Labels.Priority = "*"
	crd.Spec.Execution.Bundle = quayIoKubernautCicdTest
	crd.Spec.Parameters = []models.WorkflowParameter{
		{Name: "TARGET_NAMESPACE", Type: "string", Required: true, Description: "Namespace of the affected resource"},
	}
	return testutil.MarshalWorkflowCRD(crd)
}

// depTestUniqueWorkflowName generates a per-process, per-call unique workflow
// name (GinkgoParallelProcess() + UUID), matching the convention used
// throughout this suite (see suite_test.go's uniqueTestID, workflow_repository_
// and workflow_discovery_repository_test.go) to let specs run safely across
// parallel Ginkgo processes without colliding on the catalog's
// workflow_name/version uniqueness constraint.
func depTestUniqueWorkflowName(suffix string) string {
	return fmt.Sprintf("dep-test-workflow-%s-%d-%s", suffix, GinkgoParallelProcess(), uuid.New().String())
}

func depTestSchemaWithSecrets(workflowName string, secretNames ...string) string {
	crd := testutil.NewTestWorkflowCRD(workflowName, "RestartPod", "job")
	crd.Spec.Description = sharedtypes.StructuredDescription{
		What:      "Integration test workflow for dependency validation removal (#1481)",
		WhenToUse: "When testing Issue #1481",
	}
	crd.Spec.Labels.Severity = []string{"critical"}
	crd.Spec.Labels.Environment = []string{"*"}
	crd.Spec.Labels.Component = []string{"apps/v1/Deployment"}
	crd.Spec.Labels.Priority = "*"
	crd.Spec.Execution.Bundle = quayIoKubernautCicdTest
	crd.Spec.Parameters = []models.WorkflowParameter{
		{Name: "TARGET_NAMESPACE", Type: "string", Required: true, Description: "Namespace of the affected resource"},
	}
	if len(secretNames) > 0 {
		deps := &models.WorkflowDependencies{}
		for _, name := range secretNames {
			deps.Secrets = append(deps.Secrets, models.ResourceDependency{Name: name})
		}
		crd.Spec.Dependencies = deps
	}
	return testutil.MarshalWorkflowCRD(crd)
}

func depTestSchemaWithConfigMaps(workflowName string, cmNames ...string) string {
	crd := testutil.NewTestWorkflowCRD(workflowName, "RestartPod", "job")
	crd.Spec.Description = sharedtypes.StructuredDescription{
		What:      "Integration test workflow for dependency validation removal (#1481)",
		WhenToUse: "When testing Issue #1481",
	}
	crd.Spec.Labels.Severity = []string{"critical"}
	crd.Spec.Labels.Environment = []string{"*"}
	crd.Spec.Labels.Component = []string{"apps/v1/Deployment"}
	crd.Spec.Labels.Priority = "*"
	crd.Spec.Execution.Bundle = quayIoKubernautCicdTest
	crd.Spec.Parameters = []models.WorkflowParameter{
		{Name: "TARGET_NAMESPACE", Type: "string", Required: true, Description: "Namespace of the affected resource"},
	}
	if len(cmNames) > 0 {
		deps := &models.WorkflowDependencies{}
		for _, name := range cmNames {
			deps.ConfigMaps = append(deps.ConfigMaps, models.ResourceDependency{Name: name})
		}
		crd.Spec.Dependencies = deps
	}
	return testutil.MarshalWorkflowCRD(crd)
}

// createDepTestServer creates an in-process DS server. Issue #1481: no
// dependency validator is wired any more — schema-declared dependencies flow
// straight into the catalog without any existence check.
func createDepTestServer(schemaYAML string) (*httptest.Server, *server.Server) {
	serverCfg := &server.Config{
		Port:         18090,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	pgHost := os.Getenv("POSTGRES_HOST")
	if pgHost == "" {
		pgHost = localhost
	}
	pgPort := os.Getenv("POSTGRES_PORT")
	if pgPort == "" {
		pgPort = "5433"
	}
	dbConnStr := fmt.Sprintf("host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable", pgHost, pgPort)

	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = localhost
	}
	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = "6379"
	}
	redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort)

	appCfg := &config.Config{
		Server: config.ServerConfig{
			SignerCertDir: datastorageIntegrationSigningCertDirOrDie(),
		},
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

func registerWorkflow(serverURL, schemaContent string) (*http.Response, error) {
	body := fmt.Sprintf(`{"content":%s}`, jsonEscapeDepTest(schemaContent))
	req, err := http.NewRequest("POST", serverURL+"/api/v1/workflows", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

func jsonEscapeDepTest(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

var _ = Describe("Issue #1481: Dependency Validation Removed from Registration", Label("integration", "issue-1481"), func() {

	Context("Registration no longer checks K8s for dependency existence", func() {

		It("IT-DS-1481-001: should accept workflow when all declared secrets exist with data", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "it-gitea-creds-001", Namespace: depTestNamespace},
				Data:       map[string][]byte{"username": []byte("kubernaut"), "password": []byte("s3cret")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer deleteK8sObject(secret)

			schemaYAML := depTestSchemaWithSecrets(depTestUniqueWorkflowName("001"), "it-gitea-creds-001")
			testServer, srv := createDepTestServer(schemaYAML)
			defer testServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			resp, err := registerWorkflow(testServer.URL, schemaYAML)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"registration should succeed when declared secret exists with data")
		})

		It("IT-DS-1481-002: should accept workflow even when declared Secret does not exist (#1481)", func() {
			schemaYAML := depTestSchemaWithSecrets(depTestUniqueWorkflowName("002"), "it-missing-secret-002")
			testServer, srv := createDepTestServer(schemaYAML)
			defer testServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			resp, err := registerWorkflow(testServer.URL, schemaYAML)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"#1481: a missing Secret must no longer block registration; K8s validates at runtime")
		})

		It("IT-DS-1481-003: should accept workflow even when declared ConfigMap does not exist (#1481)", func() {
			schemaYAML := depTestSchemaWithConfigMaps(depTestUniqueWorkflowName("003"), "it-missing-cm-003")
			testServer, srv := createDepTestServer(schemaYAML)
			defer testServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			resp, err := registerWorkflow(testServer.URL, schemaYAML)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"#1481: a missing ConfigMap must no longer block registration; K8s validates at runtime")
		})

		It("IT-DS-1481-004: should accept workflow without dependencies section", func() {
			schemaYAML := depTestBaseSchemaUnique()
			testServer, srv := createDepTestServer(schemaYAML)
			defer testServer.Close()
			defer func() { _ = srv.Shutdown(ctx) }()

			resp, err := registerWorkflow(testServer.URL, schemaYAML)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"registration should succeed for workflows without dependencies")
		})
	})
})
