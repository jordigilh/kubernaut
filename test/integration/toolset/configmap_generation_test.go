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

package toolset_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/pkg/toolset/discovery"
	"github.com/jordigilh/kubernaut/pkg/toolset/server"
)

var _ = Describe("ConfigMap Generation Integration", func() {
	var (
		ctx          context.Context
		cancel       context.CancelFunc
		fakeClient   *fake.Clientset
		testNS       string
		serverConfig *server.Config
		srv          *server.Server
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		testNS = "test-namespace"

		fakeClient = fake.NewSimpleClientset()

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNS,
			},
		}
		_, err := fakeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		serverConfig = &server.Config{
			Port:              8080,
			MetricsPort:       9090,
			ShutdownTimeout:   5 * time.Second,
			DiscoveryInterval: 1 * time.Second,
			Namespace:         testNS,
		}
	})

	AfterEach(func() {
		if srv != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer shutdownCancel()
			_ = srv.Shutdown(shutdownCtx)
		}
		if cancel != nil {
			cancel()
		}
	})

	It("should discover a Prometheus service and generate ConfigMap", func() {
		promService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mock-prometheus",
				Namespace: testNS,
				Annotations: map[string]string{
					"kubernaut.io/toolset":      "enabled",
					"kubernaut.io/toolset-type": "prometheus",
				},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:     "http",
						Port:     9090,
						Protocol: corev1.ProtocolTCP,
					},
				},
				ClusterIP: "10.96.0.100",
			},
		}
		_, err := fakeClient.CoreV1().Services(testNS).Create(ctx, promService, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		var srvErr error
		srv, srvErr = server.NewServer(serverConfig, fakeClient)
		Expect(srvErr).ToNot(HaveOccurred())

		// Override detectors with nil health checkers for integration tests
		discoverer := discovery.NewServiceDiscoverer(fakeClient, 10*time.Second)
		discoverer.RegisterDetector(discovery.NewPrometheusDetectorWithHealthChecker(nil))
		discoverer.RegisterDetector(discovery.NewGrafanaDetectorWithHealthChecker(nil))
		discoverer.RegisterDetector(discovery.NewJaegerDetectorWithHealthChecker(nil))
		discoverer.RegisterDetector(discovery.NewElasticsearchDetectorWithHealthChecker(nil))
		discoverer.RegisterDetector(discovery.NewCustomDetectorWithHealthChecker(nil))

		// Replace server's discoverer with our test one
		srv.SetDiscoverer(discoverer)

		go func() {
			defer GinkgoRecover()
			_ = srv.Start(ctx)
		}()

		time.Sleep(500 * time.Millisecond)

		Eventually(func() error {
			_, err := fakeClient.CoreV1().ConfigMaps(testNS).Get(ctx, "kubernaut-toolset-config", metav1.GetOptions{})
			return err
		}, 5*time.Second, 500*time.Millisecond).Should(Succeed())

		cm, err := fakeClient.CoreV1().ConfigMaps(testNS).Get(ctx, "kubernaut-toolset-config", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(cm.Data).To(HaveKey("toolset.json"))

		toolsetYAML := cm.Data["toolset.json"]
		Expect(toolsetYAML).To(ContainSubstring("prometheus"))
		Expect(toolsetYAML).To(ContainSubstring("mock-prometheus"))
	})
})
