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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Issue #1661 Phase 52 (Change 9, ActionType-seeding migration -- discovered
// gap): SeedActionTypesViaAPI/SeedActionTypesViaAPIWithTLS (test/infrastructure/
// actiontype_e2e.go) seed action types via DataStorage's Postgres-backed
// POST /api/v1/action-types endpoint, used by 9 test-infrastructure callers
// across nearly every E2E suite. SeedE2EActionTypes already exists as a
// CRD-based alternative, but it waits for AuthWebhook to patch
// .status.registered=true -- unusable for the 6 of those 9 callers (Gateway,
// AIAnalysis, APIFrontend, KA, SignalProcessing, WorkflowExecution-bundles
// E2E suites) that don't deploy AuthWebhook at all, because they test their
// own component, not AW's admission path.
//
// SeedActionTypesViaCRD is the AuthWebhook-independent alternative: it
// creates the ActionType CRD directly (no admission webhook required) and
// relies on DataStorage's own informer-backed cache (workflowcache.Cache,
// Phase 28-30) observing the raw object. IT-DS-1661-P29-001
// (server_workflow_cache_wiring_test.go) already proves this mechanism
// generically for RemediationWorkflow; this RED test proves the same holds
// for ActionType, through the actual seeding helper test infrastructure will
// call (not just the underlying cache primitive).
//
// Business Requirements: BR-WORKFLOW-007 (ActionType CRD lifecycle), DD-WORKFLOW-016.
var _ = Describe("IT-DS-1661-P52 ActionType CRD seeding (AuthWebhook-independent)", Label("integration", "datastorage", "workflow-cache"), func() {

	buildActionTypeSeedServerDeps := func() server.ServerDeps {
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
				Port:         18092,
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
			},
			DLQMaxLen:     100,
			Authenticator: &auth.MockAuthenticator{},
			Authorizer:    &auth.MockAuthorizer{},
			AuthNamespace: "datastorage-test",
			K8sRestConfig: dsK8sRestConfig,
		}
	}

	It("IT-DS-1661-P52-001: SeedActionTypesViaCRD makes action types visible in DS's cache with zero AuthWebhook involvement", func() {
		srv, err := server.NewServer(buildActionTypeSeedServerDeps())
		Expect(err).ToNot(HaveOccurred(), "server should build successfully with a real K8s rest.Config")
		DeferCleanup(func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		})
		Expect(srv.WorkflowCache()).ToNot(BeNil(), "workflow cache must be built for this test to be meaningful")

		namespace := fmt.Sprintf("it-1661-p52-%d", time.Now().UnixNano())
		nsObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, nsObj)).To(Succeed())
		// #1661: deleting the Namespace object alone does NOT clean up the
		// ActionType CRDs seeded into it below. envtest runs only
		// etcd+kube-apiserver -- there is no namespace-lifecycle/GC
		// controller to cascade-delete a namespace's contents once it's
		// marked Terminating, so those 11 canonical-named ActionType CRDs
		// (spec.name: ScaleReplicas, RestartPod, ...) would otherwise leak
		// into this package's shared dsK8sRestConfig cluster for the rest of
		// the suite run. workflowcache.Cache.ListActionTypes lists
		// cluster-wide with no namespace filter, so every one of these
		// leaked-but-Active duplicates makes listActionsFromCache
		// (discovery_cache.go) emit a second ActionTypeEntry for the same
		// spec.Name, silently doubling every Serial spec's totalCount in
		// workflow_discovery_repository_test.go, _cluster_test.go and
		// _case_insensitive_test.go (root-caused for the #1661 "exactly 2x"
		// totalCount failures -- e.g. expected 1 got 2, expected 5 got 10).
		// Delete the ActionType CRDs directly (individual object deletes work
		// fine on envtest, unlike namespace cascade-deletion) and wait for
		// sharedWorkflowCache -- the suite-wide cache the Serial specs read
		// through -- to observe their removal before this spec is considered
		// finished.
		DeferCleanup(func() {
			var list atv1alpha1.ActionTypeList
			Expect(k8sClient.List(ctx, &list, client.InNamespace(namespace))).To(Succeed(),
				"list ActionTypes in %s for cleanup", namespace)
			for i := range list.Items {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &list.Items[i]))).To(Succeed(),
					"delete ActionType %s/%s", namespace, list.Items[i].Name)
			}
			Eventually(func() (int, error) {
				cached, cacheErr := sharedWorkflowCache.ListActionTypes(ctx)
				if cacheErr != nil {
					return -1, cacheErr
				}
				leaked := 0
				for _, at := range cached {
					if at.Namespace == namespace {
						leaked++
					}
				}
				return leaked, nil
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(0),
				"sharedWorkflowCache must observe all ActionTypes in %s deleted before the next spec runs", namespace)

			_ = k8sClient.Delete(ctx, nsObj)
		})

		kubeconfigPath, err := infrastructure.WriteEnvtestKubeconfigToFile(dsK8sRestConfig, "it-1661-p52")
		Expect(err).ToNot(HaveOccurred(), "writing envtest kubeconfig for SeedActionTypesViaCRD")

		// No AuthWebhook deployed anywhere in this suite -- proves the helper
		// does not depend on AW admission to make action types discoverable.
		Expect(infrastructure.SeedActionTypesViaCRD(ctx, kubeconfigPath, namespace, GinkgoWriter)).To(Succeed())

		Eventually(func() bool {
			got, getErr := srv.WorkflowCache().GetActionType(ctx, "RestartPod")
			Expect(getErr).ToNot(HaveOccurred())
			return got != nil
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(),
			"ActionType created via SeedActionTypesViaCRD (no AuthWebhook) must be observed by DS's cache directly")
	})
})
