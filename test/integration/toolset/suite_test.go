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

package toolset

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"

	"github.com/jordigilh/kubernaut/pkg/testutil/kind"
)

// Test suite variables
var (
	suite     *kind.IntegrationSuite
	k8sClient kubernetes.Interface
	ctx       context.Context
	cancel    context.CancelFunc
	testRunID string // Unique ID for this test run to avoid namespace collisions
)

// getUniqueNamespace generates a unique namespace name to avoid collisions
// Format: {baseName}-{testRunID}-{nanosecondTimestamp}
// Uses nanoseconds to prevent race conditions with async namespace deletion
func getUniqueNamespace(baseName string) string {
	return fmt.Sprintf("%s-%s-%d", baseName, testRunID, time.Now().UnixNano())
}

func TestToolsetIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dynamic Toolset Integration Suite (KIND)")
}

var _ = BeforeSuite(func() {
	// Generate unique test run ID
	testRunID = time.Now().Format("20060102-150405")
	GinkgoWriter.Printf("🔑 Test Run ID: %s\n", testRunID)

	ctx, cancel = context.WithCancel(context.Background())

	By("connecting to existing KIND cluster")

	// Connect to existing KIND cluster (created via `make bootstrap-dev`)
	// Creates namespaces for test isolation
	suite = kind.Setup("monitoring", "observability", "kubernaut-system")
	k8sClient = suite.Client

	GinkgoWriter.Println("✅ KIND cluster ready for direct discoverer tests")
	GinkgoWriter.Println("✅ Dynamic Toolset integration test environment ready (no server)")
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")

	// Cancel context
	cancel()

	// Cleanup test namespaces and resources
	if suite != nil {
		suite.Cleanup()
	}

	GinkgoWriter.Println("✅ Cleanup complete")
})

// createStandardTestServices creates the standard set of test services expected by service_discovery_test.go
// These services are created per-test for isolation and cleaned up in AfterEach
//
// Services created:
// - prometheus-server (monitoring namespace, port 9090)
// - grafana (monitoring namespace, port 3000)
// - jaeger-query (observability namespace, port 16686)
// - elasticsearch (observability namespace, port 9200)
// - custom-toolset-service (default namespace, port 8080)
func createStandardTestServices(ctx context.Context, client kubernetes.Interface) {
	services := []struct {
		namespace   string
		name        string
		labels      map[string]string
		port        int32
		annotations map[string]string
	}{
		{
			namespace: "monitoring",
			name:      "prometheus-server",
			labels:    map[string]string{"app": "prometheus"},
			port:      9090,
		},
		{
			namespace: "monitoring",
			name:      "grafana",
			labels:    map[string]string{"app": "grafana"},
			port:      3000,
		},
		{
			namespace: "observability",
			name:      "jaeger-query",
			labels:    map[string]string{"app.kubernetes.io/name": "jaeger"},
			port:      16686,
		},
		{
			namespace: "observability",
			name:      "elasticsearch",
			labels:    map[string]string{"app": "elasticsearch"},
			port:      9200,
		},
		{
			namespace: "default",
			name:      "custom-toolset-service",
			labels:    map[string]string{},
			port:      8080,
			annotations: map[string]string{
				"kubernaut.io/toolset":      "enabled",
				"kubernaut.io/toolset-type": "custom-api",
			},
		},
	}

	for _, svcSpec := range services {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        svcSpec.name,
				Namespace:   svcSpec.namespace,
				Labels:      svcSpec.labels,
				Annotations: svcSpec.annotations,
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:       "http",
						Port:       svcSpec.port,
						TargetPort: intstr.FromInt(int(svcSpec.port)),
						Protocol:   corev1.ProtocolTCP,
					},
				},
				Selector: map[string]string{
					"app": svcSpec.name,
				},
			},
		}

		_, err := client.CoreV1().Services(svcSpec.namespace).Create(ctx, svc, metav1.CreateOptions{})
		if err != nil {
			// Service might already exist from previous test, ignore
			GinkgoWriter.Printf("⚠️  Service %s/%s creation: %v\n", svcSpec.namespace, svcSpec.name, err)
		} else {
			GinkgoWriter.Printf("✅ Created test service: %s/%s\n", svcSpec.namespace, svcSpec.name)
		}
	}
}

// cleanupStandardTestServices removes the standard test services
func cleanupStandardTestServices(ctx context.Context, client kubernetes.Interface) {
	services := []struct {
		namespace string
		name      string
	}{
		{"monitoring", "prometheus-server"},
		{"monitoring", "grafana"},
		{"observability", "jaeger-query"},
		{"observability", "elasticsearch"},
		{"default", "custom-toolset-service"},
	}

	for _, svcSpec := range services {
		err := client.CoreV1().Services(svcSpec.namespace).Delete(ctx, svcSpec.name, metav1.DeleteOptions{})
		if err != nil {
			GinkgoWriter.Printf("⚠️  Service %s/%s cleanup: %v\n", svcSpec.namespace, svcSpec.name, err)
		}
	}
}
