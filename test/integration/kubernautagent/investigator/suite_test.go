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

package investigator_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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

func TestKubernautAgentInvestigatorIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kubernaut Agent Investigator Integration Suite — #433")
}

const (
	invPostgresPort    = 13334
	invRedisPort       = 13335
	invDataStoragePort = 13336
	invMetricsPort     = 13337
)

var (
	dsInfra     *infrastructure.DSBootstrapInfra
	ogenClient  *ogenclient.Client
	seedDB      *sql.DB
	suiteLogger logr.Logger

	// Real adapters shared across tests.
	suiteK8sAdapter *enrichment.K8sAdapter
	suiteDSAdapter  enrichment.DataStorageClient
	suiteAuditStore *audit.DSAuditStore
)

// capturingAuditStore wraps a real DSAuditStore, forwarding all events to DS
// and capturing them in-memory for test assertions. This is NOT a mock — the
// real store always receives the event.
type capturingAuditStore struct {
	real   audit.AuditStore
	events []*audit.AuditEvent
}

func newCapturingAuditStore(delegate audit.AuditStore) *capturingAuditStore {
	return &capturingAuditStore{real: delegate}
}

func (c *capturingAuditStore) StoreAudit(ctx context.Context, event *audit.AuditEvent) error {
	c.events = append(c.events, event)
	return c.real.StoreAudit(ctx, event)
}

var _ audit.AuditStore = (*capturingAuditStore)(nil)

// seedAuditEvent inserts an audit event directly into PostgreSQL.
// Used by investigator ITs that need remediation history or other
// audit-sourced data to be present before the enricher runs.
func seedAuditEvent(
	ctx context.Context,
	eventType, eventCategory, correlationID string,
	eventData map[string]interface{},
	ts time.Time,
) {
	GinkgoHelper()
	eventDataJSON, err := json.Marshal(eventData)
	Expect(err).ToNot(HaveOccurred())

	_, err = seedDB.ExecContext(ctx,
		`INSERT INTO audit_events (
			event_id, event_date, event_timestamp, event_type, event_version,
			event_category, event_action, event_outcome, correlation_id,
			resource_type, resource_id, actor_id, actor_type,
			retention_days, is_sensitive, event_data
		) VALUES (
			$1, $2, $3, $4, '1.0',
			$5, 'create', 'success', $6,
			'test', 'test', 'test', 'system',
			90, false, $7
		)`,
		uuid.New().String(), ts.Format("2006-01-02"), ts, eventType,
		eventCategory, correlationID, eventDataJSON,
	)
	Expect(err).ToNot(HaveOccurred(), "seedAuditEvent INSERT should succeed")
}

var _ = SynchronizedBeforeSuite(
	func() []byte {
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("KA Investigator IT - PHASE 1: Infrastructure Setup")
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		By("Starting envtest")
		sharedTestEnv := &envtest.Environment{
			CRDDirectoryPaths:     []string{"../../../../config/crd/bases"},
			ErrorIfCRDPathMissing: true,
		}
		sharedK8sConfig, err := sharedTestEnv.Start()
		Expect(err).ToNot(HaveOccurred(), "envtest should start")
		GinkgoWriter.Printf("envtest started at %s\n", sharedK8sConfig.Host)

		kubeconfigPath, err := infrastructure.WriteEnvtestKubeconfigToFile(sharedK8sConfig, "ka-investigator")
		Expect(err).ToNot(HaveOccurred())

		By("Creating ServiceAccount for DataStorage authentication")
		authConfig, err := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(
			sharedK8sConfig, "ka-investigator-sa", "default", GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred())

		By("Starting DataStorage infrastructure (PostgreSQL, Redis, DataStorage)")
		cfg := infrastructure.NewDSBootstrapConfigWithAuth(
			"kainvestigator",
			invPostgresPort, invRedisPort, invDataStoragePort, invMetricsPort,
			"test/integration/kubernautagent/investigator/config",
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

		suiteLogger = logr.Discard()
		dsURL := fmt.Sprintf("http://127.0.0.1:%d", invDataStoragePort)
		dsClients := integration.NewAuthenticatedDataStorageClients(dsURL, dsToken, 10*time.Second)
		ogenClient = dsClients.OpenAPIClient

		suiteDSAdapter = enrichment.NewDSAdapter(ogenClient)
		suiteAuditStore = audit.NewDSAuditStore(ogenClient)

		k8sConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "kubeconfig should be loadable")

		dynClient, err := dynamic.NewForConfig(k8sConfig)
		Expect(err).ToNot(HaveOccurred())

		discoveryClient, err := discovery.NewDiscoveryClientForConfig(k8sConfig)
		Expect(err).ToNot(HaveOccurred())

		groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
		Expect(err).ToNot(HaveOccurred(), "API group resources should be discoverable")

		discoveryMapper := restmapper.NewDiscoveryRESTMapper(groupResources)
		suiteK8sAdapter = enrichment.NewK8sAdapter(dynClient, discoveryMapper)
		suiteK8sAdapter.SetLogger(suiteLogger.WithName("k8s-adapter"))

		connStr := fmt.Sprintf("host=127.0.0.1 port=%d user=slm_user password=test_password dbname=action_history sslmode=disable", invPostgresPort)
		seedDB, err = sql.Open("pgx", connStr)
		Expect(err).ToNot(HaveOccurred(), "direct PostgreSQL connection should open")
		Expect(seedDB.Ping()).To(Succeed(), "PostgreSQL should be reachable")

		GinkgoWriter.Println("Phase 2 complete - K8s adapter, DS adapter, audit store, seedDB ready")
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
		GinkgoWriter.Println("KA Investigator IT - Infrastructure Cleanup")
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		if dsInfra != nil {
			infrastructure.MustGatherContainerLogs("kainvestigator", []string{
				dsInfra.DataStorageContainer,
				dsInfra.PostgresContainer,
				dsInfra.RedisContainer,
			}, GinkgoWriter)
			_ = infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
		}
		GinkgoWriter.Println("Suite complete")
	},
)

// createK8sFixtures populates envtest with the Deployment → ReplicaSet → Pod
// hierarchy used by most investigator tests. The namespace "production" is the
// standard fixture namespace matching existing test signals.
func createK8sFixtures(dynClient dynamic.Interface) {
	GinkgoHelper()
	ctx := context.Background()

	nsGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	for _, ns := range []string{"production", "demo-quota", "constrained"} {
		nsObj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata":   map[string]interface{}{"name": ns},
			},
		}
		_, err := dynClient.Resource(nsGVR).Create(ctx, nsObj, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred(), "namespace %s creation should succeed", ns)
	}

	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	rsGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	podGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

	type fixture struct {
		namespace  string
		deployName string
		rsName     string
		podNames   []string
	}

	fixtures := []fixture{
		{namespace: "production", deployName: "api-server", rsName: "api-server-abc", podNames: []string{"api-server-abc-xyz"}},
		{namespace: "production", deployName: "web-app", rsName: "web-app-rs", podNames: []string{"web-app-xyz"}},
		{namespace: "production", deployName: "meshed-app", rsName: "meshed-app-rs", podNames: []string{"meshed-app-pod"}},
		{namespace: "demo-quota", deployName: "api-server", rsName: "api-server-dq", podNames: []string{"api-server-dq-pod"}},
		{namespace: "constrained", deployName: "web-app", rsName: "web-app-c-rs", podNames: []string{"web-app-c-pod"}},
	}

	for _, f := range fixtures {
		deploy := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      f.deployName,
					"namespace": f.namespace,
				},
				"spec": map[string]interface{}{
					"replicas": int64(1),
					"selector": map[string]interface{}{
						"matchLabels": map[string]interface{}{"app": f.deployName},
					},
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{"labels": map[string]interface{}{"app": f.deployName}},
						"spec":     map[string]interface{}{"containers": []interface{}{map[string]interface{}{"name": f.deployName, "image": "nginx:latest"}}},
					},
				},
			},
		}
		created, err := dynClient.Resource(deployGVR).Namespace(f.namespace).Create(ctx, deploy, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred(), "deploy %s/%s creation", f.namespace, f.deployName)
		deployUID := string(created.GetUID())

		rs := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "ReplicaSet",
				"metadata": map[string]interface{}{
					"name":      f.rsName,
					"namespace": f.namespace,
					"ownerReferences": []interface{}{
						map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
							"name":       f.deployName,
							"uid":        deployUID,
							"controller": true,
						},
					},
				},
				"spec": map[string]interface{}{
					"replicas": int64(1),
					"selector": map[string]interface{}{
						"matchLabels": map[string]interface{}{"app": f.deployName},
					},
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{"labels": map[string]interface{}{"app": f.deployName}},
						"spec":     map[string]interface{}{"containers": []interface{}{map[string]interface{}{"name": f.deployName, "image": "nginx:latest"}}},
					},
				},
			},
		}
		createdRS, err := dynClient.Resource(rsGVR).Namespace(f.namespace).Create(ctx, rs, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred(), "rs %s/%s creation", f.namespace, f.rsName)
		rsUID := string(createdRS.GetUID())

		for _, podName := range f.podNames {
			pod := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name":      podName,
						"namespace": f.namespace,
						"ownerReferences": []interface{}{
							map[string]interface{}{
								"apiVersion": "apps/v1",
								"kind":       "ReplicaSet",
								"name":       f.rsName,
								"uid":        rsUID,
								"controller": true,
							},
						},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{map[string]interface{}{"name": f.deployName, "image": "nginx:latest"}},
					},
				},
			}
			_, err = dynClient.Resource(podGVR).Namespace(f.namespace).Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred(), "pod %s/%s creation", f.namespace, podName)
		}
	}

	GinkgoWriter.Println("K8s fixtures created: Deployment → ReplicaSet → Pod hierarchies in production, demo-quota, constrained")
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
