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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// Issue #1661 Phase A5: replaces actiontype_lifecycle_test.go (deleted --
// its IT-AT-300-.../IT-AT-512-... coverage exercised
// Create/GetByName/UpdateDescription/Disable/ForceDisable/ListActive/
// CountActiveWorkflows directly on the Postgres-backed actiontype
// repository, all of which were deleted in Phase A3/A4: AuthWebhook now owns
// the ActionType CRD lifecycle entirely locally, and DataStorage's role is
// read-only cache observation, not CRUD). The one behavior that survived
// the migration in a different shape -- counting active workflows for an
// action type -- is covered here against its new implementation:
// HandleGetActionTypeWorkflowCount reading the informer-backed workflowCache
// instead of running `SELECT ... FROM remediation_workflow_catalog`
// (DD-WORKFLOW-018, Phase A1 port).
//
// Business Requirements: BR-WORKFLOW-007 (ActionType workflow-count query).
var _ = Describe("IT-DS-1661-PA5 ActionType workflow-count (cache-backed)", Label("integration", "actiontype", "workflow-cache"), func() {

	const (
		testToken = "it-pa5-token"
		testUser  = "system:serviceaccount:datastorage-test:it-pa5"
	)

	buildWorkflowCountServerDeps := func() server.ServerDeps {
		pgHost := os.Getenv("POSTGRES_HOST")
		if pgHost == "" {
			pgHost = localhost
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
			redisHost = localhost
		}
		redisPort := os.Getenv("REDIS_PORT")
		if redisPort == "" {
			redisPort = "16379"
		}

		return server.ServerDeps{
			DBConnStr:     dbConnStr,
			RedisAddr:     fmt.Sprintf("%s:%s", redisHost, redisPort),
			RedisPassword: "",
			Logger:        logger,
			AppConfig: &config.Config{
				Server: config.ServerConfig{
					SignerCertDir: datastorageIntegrationSigningCertDirOrDie(),
				},
				Database: config.DatabaseConfig{
					MaxOpenConns:    5,
					MaxIdleConns:    2,
					ConnMaxLifetime: "1m",
					ConnMaxIdleTime: "1m",
				},
			},
			ServerConfig: &server.Config{
				Port:         18093,
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
			AuthNamespace: "datastorage-test",
			K8sRestConfig: dsK8sRestConfig,
		}
	}

	newWorkflowFixture := func(name, actionType string) *rwv1alpha1.RemediationWorkflow {
		return &rwv1alpha1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: rwv1alpha1.RemediationWorkflowSpec{
				Version:    "1.0.0",
				ActionType: actionType,
				Description: rwv1alpha1.RemediationWorkflowDescription{
					What:      "IT-DS-1661-PA5 workflow-count test fixture",
					WhenToUse: "For workflow-count cache-backed integration testing",
				},
				Labels: rwv1alpha1.RemediationWorkflowLabels{
					Severity:    []string{"critical"},
					Environment: []string{"production"},
					Component:   []string{"v1/Pod"},
					Priority:    "P1",
				},
				Execution: rwv1alpha1.RemediationWorkflowExecution{
					Engine: "job",
					Bundle: testutil.ValidBundleRef,
				},
				Parameters: []rwv1alpha1.RemediationWorkflowParameter{
					{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
				},
			},
		}
	}

	// createWorkflowWithCatalogStatus creates a RemediationWorkflow CRD and
	// patches .status.catalogStatus directly (mimicking AuthWebhook's
	// admission-time status patch, since AuthWebhook is not deployed in this
	// suite) so the cache-backed handler sees the desired lifecycle state.
	createWorkflowWithCatalogStatus := func(name, actionType string, catalogStatus sharedtypes.CatalogStatus) {
		rw := newWorkflowFixture(name, actionType)
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rw) })

		rw.Status.CatalogStatus = catalogStatus
		Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())
	}

	queryWorkflowCount := func(baseURL, actionType string) int {
		GinkgoHelper()
		req, err := http.NewRequest(http.MethodGet,
			fmt.Sprintf("%s/api/v1/action-types/%s/workflow-count", baseURL, actionType), nil)
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Authorization", "Bearer "+testToken)

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var body struct {
			Count int `json:"count"`
		}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
		return body.Count
	}

	It("IT-DS-1661-PA5-001: counts only Active workflows for the requested action type", func() {
		srv, err := server.NewServer(buildWorkflowCountServerDeps())
		Expect(err).ToNot(HaveOccurred(), "server should build successfully with a real K8s rest.Config")
		DeferCleanup(func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		})
		Expect(srv.WorkflowCache()).ToNot(BeNil(), "workflow cache must be built for this test to be meaningful")

		testServer := httptest.NewServer(srv.Handler())
		DeferCleanup(testServer.Close)

		suffix := time.Now().UnixNano()
		actionType := fmt.Sprintf("PA5CountAction%d", suffix)
		otherActionType := fmt.Sprintf("PA5OtherAction%d", suffix)

		createWorkflowWithCatalogStatus(fmt.Sprintf("pa5-active-1-%d", suffix), actionType, sharedtypes.CatalogStatusActive)
		createWorkflowWithCatalogStatus(fmt.Sprintf("pa5-active-2-%d", suffix), actionType, sharedtypes.CatalogStatusActive)
		createWorkflowWithCatalogStatus(fmt.Sprintf("pa5-disabled-%d", suffix), actionType, sharedtypes.CatalogStatusDisabled)
		createWorkflowWithCatalogStatus(fmt.Sprintf("pa5-other-type-%d", suffix), otherActionType, sharedtypes.CatalogStatusActive)

		Eventually(func() int {
			return queryWorkflowCount(testServer.URL, actionType)
		}, 5*time.Second, 100*time.Millisecond).Should(Equal(2),
			"only the 2 Active workflows for actionType should be counted -- the Disabled workflow "+
				"and the workflow registered under a different action type must both be excluded")

		Expect(queryWorkflowCount(testServer.URL, otherActionType)).To(Equal(1),
			"otherActionType has exactly one Active workflow")
	})

	It("IT-DS-1661-PA5-002: returns zero for an action type with no workflows", func() {
		srv, err := server.NewServer(buildWorkflowCountServerDeps())
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		})

		testServer := httptest.NewServer(srv.Handler())
		DeferCleanup(testServer.Close)

		Expect(queryWorkflowCount(testServer.URL, fmt.Sprintf("PA5NoSuchAction%d", time.Now().UnixNano()))).To(Equal(0))
	})
})
