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

package fleet_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrl "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
	spmetrics "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// IT-SP-054-001/002: Signal Processing Remote Enrichment via MCP Gateway
// Proves the wiring path: K8sEnricher → ReaderFactory → mcpclient → MCP Gateway
var _ = Describe("SP Remote Enrichment via MCP Gateway (BR-INTEGRATION-054)", func() {
	var (
		ctx context.Context
		gw  *mockgw.MockGateway
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	})

	AfterEach(func() {
		if gw != nil {
			gw.Close()
		}
	})

	It("IT-SP-054-001: enriches a remote cluster signal through MCP Gateway", func() {
		gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east-1"))

		mcpClient, err := mcpclient.New(ctx, gw.URL())
		Expect(err).ToNot(HaveOccurred())
		defer mcpClient.Close()

		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		localNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "prod"},
		}
		localClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(localNS).Build()

		readerFactory := enricher.NewMCPReaderFactory(localClient, mcpClient.Session())

		reg := prometheus.NewRegistry()
		m := spmetrics.NewMetricsWithRegistry(reg)
		logger := zap.New(zap.UseDevMode(true))

		k8sEnricher := enricher.NewK8sEnricher(localClient, nil, logger, m, 10*time.Second, 30*time.Second)
		k8sEnricher.SetReaderFactory(readerFactory)

		signal := &signalprocessingv1.SignalData{
			ClusterID: "prod-east-1",
			TargetResource: signalprocessingv1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "api-server",
				Namespace: "prod",
			},
		}

		result, err := k8sEnricher.Enrich(ctx, signal)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.ClusterID).To(Equal("prod-east-1"),
			"IT-SP-054-001: ClusterID must be set on the result")
		Expect(result.Workload).ToNot(BeNil(),
			"IT-SP-054-001: workload metadata from remote cluster must be populated")
		Expect(result.Workload.Name).To(Equal("api-server"))
		Expect(result.Workload.Kind).To(Equal("Deployment"))

		calls := gw.CallLog()
		Expect(calls).ToNot(BeEmpty(),
			"IT-SP-054-001: MCP Gateway must have received tool calls")
	})

	It("IT-SP-054-002: falls back to local enrichment when ClusterID is empty", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		localNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "default",
				Labels: map[string]string{"env": "local"},
			},
		}
		localClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(localNS).Build()

		readerFactory := enricher.NewLocalReaderFactory(localClient)

		reg := prometheus.NewRegistry()
		m := spmetrics.NewMetricsWithRegistry(reg)
		logger := zap.New(zap.UseDevMode(true))

		k8sEnricher := enricher.NewK8sEnricher(localClient, nil, logger, m, 10*time.Second, 30*time.Second)
		k8sEnricher.SetReaderFactory(readerFactory)

		signal := &signalprocessingv1.SignalData{
			TargetResource: signalprocessingv1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "web-pod",
				Namespace: "default",
			},
		}

		result, err := k8sEnricher.Enrich(ctx, signal)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.DegradedMode).To(BeTrue(),
			"IT-SP-054-002: pod not found in local fake client → degraded mode")
	})
})
