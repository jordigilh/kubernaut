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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	controller "github.com/jordigilh/kubernaut/internal/controller/effectivenessmonitor"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
	"github.com/jordigilh/kubernaut/pkg/fleet/fleettest"
)

var _ = Describe("EM Fleet Routing Integration (BR-FLEET-054)", func() {

	var (
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	// ========================================
	// IT-EM-054-001: ReaderFor routes to fleet reader for remote ClusterID [AC-3]
	// ========================================
	//
	// This test proves the reconciler's ReaderFor wiring: when a StubReaderFactory
	// is configured with a remote reader for "prod-east-1", ReaderFor returns that
	// reader instead of the local client. This is the integration entry point that
	// all target-facing methods (health, hash, alert) go through.
	Describe("IT-EM-054-001: ReaderFor routes remote ClusterID to fleet reader", func() {
		It("should return the fleet reader when ClusterID matches a configured cluster", func() {
			ctx := context.Background()

			remotePod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "remote-pod",
					Namespace: "production",
					Labels:    map[string]string{"app": "payment-svc"},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			}
			remoteReader := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(remotePod).
				Build()

			localReader := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			factory := &fleettest.StubReaderFactory{
				Readers: map[string]client.Reader{
					"prod-east-1": remoteReader,
				},
			}

			registry := prometheus.NewPedanticRegistry()
			m := emmetrics.NewMetricsWithRegistry(registry)

			rec := controller.NewReconciler(
				localReader,
				localReader,
				scheme,
				nil,
				m,
				nil, nil, nil, nil,
				controller.DefaultReconcilerConfig(),
			)
			rec.SetReaderFactory(factory)

			By("ReaderFor with remote ClusterID returns fleet reader")
			reader, err := rec.ReaderFor(ctx, "prod-east-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(reader).To(Equal(remoteReader))

			By("Fleet reader can read remote Pod")
			pod := &corev1.Pod{}
			err = reader.Get(ctx, client.ObjectKey{Name: "remote-pod", Namespace: "production"}, pod)
			Expect(err).ToNot(HaveOccurred())
			Expect(pod.Name).To(Equal("remote-pod"))
			Expect(pod.Labels["app"]).To(Equal("payment-svc"))

			By("Local reader cannot see the remote Pod")
			localPod := &corev1.Pod{}
			err = localReader.Get(ctx, client.ObjectKey{Name: "remote-pod", Namespace: "production"}, localPod)
			Expect(err).To(HaveOccurred(), "local reader should not have remote pod")
		})
	})

	// ========================================
	// IT-EM-054-002: ReaderFor returns local reader when ClusterID is empty [AC-3]
	// ========================================
	Describe("IT-EM-054-002: ReaderFor returns local reader for empty ClusterID", func() {
		It("should return the local client when ClusterID is empty", func() {
			ctx := context.Background()

			localPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "local-pod",
					Namespace: "default",
					Labels:    map[string]string{"app": "web"},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			}
			localClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(localPod).
				Build()

			factory := &fleettest.StubReaderFactory{
				Readers: map[string]client.Reader{},
			}

			registry := prometheus.NewPedanticRegistry()
			m := emmetrics.NewMetricsWithRegistry(registry)

			rec := controller.NewReconciler(
				localClient,
				localClient,
				scheme,
				nil,
				m,
				nil, nil, nil, nil,
				controller.DefaultReconcilerConfig(),
			)
			rec.SetReaderFactory(factory)

			reader, err := rec.ReaderFor(ctx, "")
			Expect(err).ToNot(HaveOccurred())

			pod := &corev1.Pod{}
			err = reader.Get(ctx, client.ObjectKey{Name: "local-pod", Namespace: "default"}, pod)
			Expect(err).ToNot(HaveOccurred())
			Expect(pod.Name).To(Equal("local-pod"))
		})
	})

	// ========================================
	// IT-EM-054-003: ReaderFor degrades gracefully for unknown cluster [AC-3]
	// ========================================
	Describe("IT-EM-054-003: ReaderFor returns error for unknown ClusterID", func() {
		It("should return an error when the cluster is not registered in the factory", func() {
			ctx := context.Background()

			factory := &fleettest.StubReaderFactory{
				Readers: map[string]client.Reader{
					"prod-east-1": fake.NewClientBuilder().Build(),
				},
			}

			registry := prometheus.NewPedanticRegistry()
			m := emmetrics.NewMetricsWithRegistry(registry)

			rec := controller.NewReconciler(
				fake.NewClientBuilder().Build(),
				fake.NewClientBuilder().Build(),
				scheme,
				nil,
				m,
				nil, nil, nil, nil,
				controller.DefaultReconcilerConfig(),
			)
			rec.SetReaderFactory(factory)

			_, err := rec.ReaderFor(ctx, "unknown-cluster")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown cluster"))
		})
	})
})
