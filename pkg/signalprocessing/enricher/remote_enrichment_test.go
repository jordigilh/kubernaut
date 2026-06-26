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

package enricher_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
	spmetrics "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
)

// fakeReaderFactory implements fleet.ReaderFactory for testing.
type fakeReaderFactory struct {
	readers map[string]client.Reader
}

func (f *fakeReaderFactory) ReaderFor(_ context.Context, clusterID string) (client.Reader, error) {
	if clusterID == "" {
		return nil, fmt.Errorf("fakeReaderFactory: empty clusterID should use local path")
	}
	r, ok := f.readers[clusterID]
	if !ok {
		return nil, fmt.Errorf("fakeReaderFactory: unknown cluster %q", clusterID)
	}
	return r, nil
}

var _ = Describe("K8sEnricher Remote Enrichment (BR-INTEGRATION-054)", func() {
	var (
		ctx         context.Context
		localScheme *runtime.Scheme
		localClient client.Client
		m           *spmetrics.Metrics
	)

	BeforeEach(func() {
		ctx = context.Background()
		localScheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(localScheme)).To(Succeed())
		Expect(appsv1.AddToScheme(localScheme)).To(Succeed())

		localNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "prod",
				Labels: map[string]string{"env": "local"},
			},
		}
		localClient = fake.NewClientBuilder().WithScheme(localScheme).WithObjects(localNS).Build()

		reg := prometheus.NewRegistry()
		m = spmetrics.NewMetricsWithRegistry(reg)
	})

	It("UT-SP-054-003a [AC-4]: routes to remote enrichment when ClusterID is set", func() {
		remoteDeploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-server",
				Namespace: "prod",
				Labels:    map[string]string{"app": "api", "tier": "backend"},
			},
		}
		remoteNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "prod",
				Labels: map[string]string{"env": "remote-prod"},
			},
		}
		remoteClient := fake.NewClientBuilder().
			WithScheme(localScheme).
			WithObjects(remoteDeploy, remoteNS).
			Build()

		factory := &fakeReaderFactory{
			readers: map[string]client.Reader{"prod-east-1": remoteClient},
		}

		logger := zap.New(zap.UseDevMode(true))
		e := enricher.NewK8sEnricher(localClient, nil, logger, m, 5*time.Second, 30*time.Second)
		// RED: SetReaderFactory does not exist yet
		e.SetReaderFactory(factory)

		signal := &signalprocessingv1.SignalData{
			ClusterID: "prod-east-1",
			TargetResource: signalprocessingv1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "api-server",
				Namespace: "prod",
			},
		}

		result, err := e.Enrich(ctx, signal)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.ClusterID).To(Equal("prod-east-1"))
		Expect(result.Workload).ToNot(BeNil())
		Expect(result.Workload.Name).To(Equal("api-server"))
		Expect(result.Workload.Kind).To(Equal("Deployment"))
		Expect(result.Workload.Labels).To(HaveKeyWithValue("app", "api"))
	})

	It("UT-SP-054-003b [AC-4]: falls back to local enrichment when ClusterID is empty", func() {
		localDeploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local-deploy",
				Namespace: "prod",
				Labels:    map[string]string{"app": "local"},
			},
		}
		localClient = fake.NewClientBuilder().
			WithScheme(localScheme).
			WithObjects(
				localDeploy,
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod", Labels: map[string]string{"env": "local"}}},
			).
			Build()

		logger := zap.New(zap.UseDevMode(true))
		e := enricher.NewK8sEnricher(localClient, nil, logger, m, 5*time.Second, 30*time.Second)
		e.SetReaderFactory(&fakeReaderFactory{})

		signal := &signalprocessingv1.SignalData{
			TargetResource: signalprocessingv1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "local-deploy",
				Namespace: "prod",
			},
		}

		result, err := e.Enrich(ctx, signal)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.Workload).ToNot(BeNil())
		Expect(result.Workload.Name).To(Equal("local-deploy"))
	})

	It("UT-SP-054-003c [AC-4]: enters degraded mode when remote reader fails", func() {
		factory := &fakeReaderFactory{
			readers: map[string]client.Reader{},
		}

		logger := zap.New(zap.UseDevMode(true))
		e := enricher.NewK8sEnricher(localClient, nil, logger, m, 5*time.Second, 30*time.Second)
		e.SetReaderFactory(factory)

		signal := &signalprocessingv1.SignalData{
			ClusterID: "nonexistent-cluster",
			TargetResource: signalprocessingv1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "api-server",
				Namespace: "prod",
			},
		}

		result, err := e.Enrich(ctx, signal)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.DegradedMode).To(BeTrue(),
			"should enter degraded mode when remote reader cannot be obtained")
		Expect(result.ClusterID).To(Equal("nonexistent-cluster"))
	})
})
