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

package enrichment_test

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	"github.com/jordigilh/kubernaut/test/shared/integration"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestKubernautAgentEnrichmentIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kubernaut Agent Enrichment Integration Suite — #433")
}

const (
	enrPostgresPort    = 13326
	enrRedisPort       = 13327
	enrDataStoragePort = 13328
	enrMetricsPort     = 13329
)

var (
	dsInfra     *infrastructure.DSBootstrapInfra
	ogenClient  *ogenclient.Client
	seedDB      *sql.DB
	enricher    *enrichment.Enricher
	auditStore  *audit.DSAuditStore
	suiteLogger *slog.Logger
	k8sAdapter  enrichment.K8sClient
)

var _ = SynchronizedBeforeSuite(
	func() []byte {
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("KA Enrichment IT - PHASE 1: Infrastructure Setup")
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		By("Starting envtest")
		sharedTestEnv := &envtest.Environment{
			CRDDirectoryPaths:     []string{"../../../../config/crd/bases"},
			ErrorIfCRDPathMissing: true,
		}
		sharedK8sConfig, err := sharedTestEnv.Start()
		Expect(err).ToNot(HaveOccurred(), "envtest should start")
		GinkgoWriter.Printf("envtest started at %s\n", sharedK8sConfig.Host)

		kubeconfigPath, err := infrastructure.WriteEnvtestKubeconfigToFile(sharedK8sConfig, "ka-enrichment")
		Expect(err).ToNot(HaveOccurred())

		By("Creating ServiceAccount for DataStorage authentication")
		authConfig, err := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(
			sharedK8sConfig, "ka-enrichment-sa", "default", GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred())

		By("Starting DataStorage infrastructure (PostgreSQL, Redis, DataStorage)")
		cfg := infrastructure.NewDSBootstrapConfigWithAuth(
			"kaenrichment",
			enrPostgresPort, enrRedisPort, enrDataStoragePort, enrMetricsPort,
			"test/integration/kubernautagent/enrichment/config",
			authConfig,
		)
		dsInfra, err = infrastructure.StartDSBootstrap(cfg, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "DS infrastructure must start")
		dsInfra.SharedTestEnv = sharedTestEnv

		By("Creating K8s fixtures for owner chain resolution")
		dynClient, err := dynamic.NewForConfig(sharedK8sConfig)
		Expect(err).ToNot(HaveOccurred())

		createK8sFixtures(dynClient)

		GinkgoWriter.Println("Phase 1 complete - DataStorage infrastructure + K8s fixtures ready")

		payload := authConfig.Token + "\n" + kubeconfigPath
		return []byte(payload)
	},

	func(data []byte) {
		lines := splitLines(string(data))
		dsToken := lines[0]
		kubeconfigPath := lines[1]

		suiteLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

		dsURL := fmt.Sprintf("http://127.0.0.1:%d", enrDataStoragePort)
		dsClients := integration.NewAuthenticatedDataStorageClients(dsURL, dsToken, 10*time.Second)
		ogenClient = dsClients.OpenAPIClient

		dsAdapter := enrichment.NewDSAdapter(ogenClient)
		auditStore = audit.NewDSAuditStore(ogenClient)

		k8sConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "kubeconfig should be loadable")

		dynClient, err := dynamic.NewForConfig(k8sConfig)
		Expect(err).ToNot(HaveOccurred())

		discoveryClient, err := discovery.NewDiscoveryClientForConfig(k8sConfig)
		Expect(err).ToNot(HaveOccurred())

		groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
		Expect(err).ToNot(HaveOccurred(), "API group resources should be discoverable")

		discoveryMapper := restmapper.NewDiscoveryRESTMapper(groupResources)
		k8sAdapter = enrichment.NewK8sAdapter(dynClient, discoveryMapper)
		enricher = enrichment.NewEnricher(k8sAdapter, dsAdapter, auditStore, suiteLogger)

		connStr := fmt.Sprintf("host=127.0.0.1 port=%d user=slm_user password=test_password dbname=action_history sslmode=disable", enrPostgresPort)
		seedDB, err = sql.Open("pgx", connStr)
		Expect(err).ToNot(HaveOccurred(), "direct PostgreSQL connection should open")
		Expect(seedDB.Ping()).To(Succeed(), "PostgreSQL should be reachable")

		GinkgoWriter.Println("Phase 2 complete - enricher, seedDB, auditStore ready")
	},
)

var _ = SynchronizedAfterSuite(
	func() {
		if seedDB != nil {
			_ = seedDB.Close()
		}
	},
	func() {
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("KA Enrichment IT - Infrastructure Cleanup")
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		if dsInfra != nil {
			infrastructure.MustGatherContainerLogs("kaenrichment", []string{
				dsInfra.DataStorageContainer,
				dsInfra.PostgresContainer,
				dsInfra.RedisContainer,
			}, GinkgoWriter)
			_ = infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
		}
		GinkgoWriter.Println("Suite complete")
	},
)

func createK8sFixtures(dynClient dynamic.Interface) {
	GinkgoHelper()
	ctx := context.Background()

	nsGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	nsObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata":   map[string]interface{}{"name": "it-enrichment"},
		},
	}
	_, err := dynClient.Resource(nsGVR).Create(ctx, nsObj, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "namespace creation should succeed")

	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	deploy := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "web-deploy",
				"namespace": "it-enrichment",
			},
			"spec": map[string]interface{}{
				"replicas": int64(1),
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{"app": "web"},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "web"},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "web",
								"image": "nginx:latest",
							},
						},
					},
				},
			},
		},
	}
	createdDeploy, err := dynClient.Resource(deployGVR).Namespace("it-enrichment").Create(ctx, deploy, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "deployment creation should succeed")
	deployUID := string(createdDeploy.GetUID())

	rsGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	rs := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "ReplicaSet",
			"metadata": map[string]interface{}{
				"name":      "web-rs-abc",
				"namespace": "it-enrichment",
			"ownerReferences": []interface{}{
				map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"name":       "web-deploy",
					"uid":        deployUID,
					"controller": true,
				},
			},
			},
			"spec": map[string]interface{}{
				"replicas": int64(1),
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{"app": "web"},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "web"},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "web",
								"image": "nginx:latest",
							},
						},
					},
				},
			},
		},
	}
	createdRS, err := dynClient.Resource(rsGVR).Namespace("it-enrichment").Create(ctx, rs, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "replicaset creation should succeed")
	rsUID := string(createdRS.GetUID())

	podGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	pod := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "web-pod-1",
				"namespace": "it-enrichment",
			"ownerReferences": []interface{}{
				map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "ReplicaSet",
					"name":       "web-rs-abc",
					"uid":        rsUID,
					"controller": true,
				},
			},
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "web",
						"image": "nginx:latest",
					},
				},
			},
		},
	}
	_, err = dynClient.Resource(podGVR).Namespace("it-enrichment").Create(ctx, pod, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "pod creation should succeed")

	GinkgoWriter.Println("K8s fixtures created: Deployment -> ReplicaSet -> Pod in it-enrichment")
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
