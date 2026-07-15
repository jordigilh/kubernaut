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
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// Issue #1661 Phase 29 (Change 5, CHECKPOINT W): proves the informer-backed
// workflow cache (pkg/datastorage/workflowcache, Phase 28) is wired into
// DataStorage's *production* server-construction path
// (server.NewServer -> buildWorkflowCache), not just constructed directly by
// a test as workflow_cache_test.go (Phase 28) does. cmd/datastorage/main.go
// calls server.NewServer with the same ServerDeps.K8sRestConfig field
// exercised here, wired from buildK8sAuthDeps's rest.Config
// (cmd/datastorage/main.go).
//
// Business Requirements: BR-WORKFLOW-006, BR-WORKFLOW-007.
var _ = Describe("IT-DS-1661-P29 Server wiring: workflow cache", Label("integration", "datastorage", "workflow-cache"), func() {

	buildServerDeps := func(extra server.ServerDeps) server.ServerDeps {
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

		const token = "it-1661-p29-token"
		const user = "system:serviceaccount:datastorage-test:it-1661-p29"

		deps := server.ServerDeps{
			DBConnStr:     dbConnStr,
			RedisAddr:     redisAddr,
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
				Port:         18091,
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
			},
			DLQMaxLen: 100,
			Authenticator: &auth.MockAuthenticator{
				ValidUsers: map[string]string{token: user},
			},
			Authorizer: &auth.MockAuthorizer{
				AllowedUsers: map[string]bool{user: true},
			},
			AuthNamespace: "datastorage-test",
		}
		deps.K8sRestConfig = extra.K8sRestConfig
		return deps
	}

	It("IT-DS-1661-P29-001: NewServer builds a functioning workflow cache when K8sRestConfig is supplied", func() {
		srv, err := server.NewServer(buildServerDeps(server.ServerDeps{K8sRestConfig: dsK8sRestConfig}))
		Expect(err).ToNot(HaveOccurred(), "server should build successfully with a real K8s rest.Config")
		DeferCleanup(func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		})

		Expect(srv.WorkflowCache()).ToNot(BeNil(),
			"WorkflowCache() must be non-nil when ServerDeps.K8sRestConfig is supplied -- proves "+
				"cmd/datastorage/main.go's production construction path wires the Phase 28 cache, "+
				"not just a direct test construction")

		name := fmt.Sprintf("it-1661-p29-%d", time.Now().UnixNano())
		rw := &rwv1alpha1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: rwv1alpha1.RemediationWorkflowSpec{
				Version:    "1.0.0",
				ActionType: "ScaleReplicas",
				Description: rwv1alpha1.RemediationWorkflowDescription{
					What:      "IT-DS-1661-P29 server-wiring test fixture",
					WhenToUse: "For workflow cache production-wiring integration testing",
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
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rw) })

		Eventually(func() bool {
			got, getErr := srv.WorkflowCache().GetWorkflow(ctx, name)
			Expect(getErr).ToNot(HaveOccurred())
			return got != nil
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(),
			"server's workflow cache (built via the production NewServer path) must observe a CRD "+
				"created directly against etcd")
	})

	It("IT-DS-1661-P29-002: NewServer leaves the workflow cache nil when K8sRestConfig is omitted (backward compatibility)", func() {
		srv, err := server.NewServer(buildServerDeps(server.ServerDeps{}))
		Expect(err).ToNot(HaveOccurred(), "server should still build successfully without a K8s rest.Config")
		DeferCleanup(func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		})

		Expect(srv.WorkflowCache()).To(BeNil(),
			"omitting K8sRestConfig must be a no-op, preserving every existing caller of server.NewServer "+
				"that does not set it")
	})
})
