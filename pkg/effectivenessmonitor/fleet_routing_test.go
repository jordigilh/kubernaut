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

package effectivenessmonitor_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/effectivenessmonitor"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
	"github.com/jordigilh/kubernaut/pkg/fleet/fleettest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Fleet Routing (BR-FLEET-054)", func() {

	// ========================================
	// UT-EM-054-001: readerFor routes remote ClusterID to ReaderFactory [AC-3]
	// ========================================
	Describe("readerFor with remote ClusterID (UT-EM-054-001)", func() {
		It("should return a fleet reader when ClusterID is non-empty and ReaderFactory is set", func() {
			remoteReader := fake.NewClientBuilder().Build()
			factory := &fleettest.StubReaderFactory{
				Readers: map[string]client.Reader{
					"prod-east-1": remoteReader,
				},
			}

			rec := controller.NewReconciler(
				fake.NewClientBuilder().Build(),
				fake.NewClientBuilder().Build(),
				nil, nil, nil, nil, nil, nil, nil,
				controller.DefaultReconcilerConfig(),
			)
			rec.SetReaderFactory(factory)

			reader, err := rec.ReaderFor(context.Background(), "prod-east-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(reader).To(Equal(remoteReader))
		})
	})

	// ========================================
	// UT-EM-054-002: readerFor returns local targetReader when ClusterID is empty [AC-3]
	// ========================================
	Describe("readerFor with empty ClusterID (UT-EM-054-002)", func() {
		It("should return the local targetReader when ClusterID is empty", func() {
			localClient := fake.NewClientBuilder().Build()

			rec := controller.NewReconciler(
				localClient,
				fake.NewClientBuilder().Build(),
				nil, nil, nil, nil, nil, nil, nil,
				controller.DefaultReconcilerConfig(),
			)

			reader, err := rec.ReaderFor(context.Background(), "")
			Expect(err).ToNot(HaveOccurred())
			Expect(reader).To(Equal(localClient))
		})
	})

	// ========================================
	// UT-EM-054-003: RecordValidityExpiration records cluster label [AU-3, SI-4]
	// ========================================
	Describe("RecordValidityExpiration with cluster label (UT-EM-054-003)", func() {
		It("should record the cluster label in the validity expiration metric", func() {
			registry := prometheus.NewPedanticRegistry()
			m := emmetrics.NewMetricsWithRegistry(registry)

			m.RecordValidityExpiration("prod-east-1")

			count := testutil.ToFloat64(m.ValidityExpirationsTotal.WithLabelValues("prod-east-1"))
			Expect(count).To(Equal(1.0))

			m.RecordValidityExpiration("")
			localCount := testutil.ToFloat64(m.ValidityExpirationsTotal.WithLabelValues(""))
			Expect(localCount).To(Equal(1.0))
		})
	})

	// ========================================
	// UT-EM-054-004: EA spec carries ClusterID [AC-4]
	// ========================================
	Describe("EA ClusterID field (UT-EM-054-004)", func() {
		It("should carry ClusterID in EA spec", func() {
			ea := &eav1.EffectivenessAssessment{
				Spec: eav1.EffectivenessAssessmentSpec{
					ClusterID:     "prod-east-1",
					CorrelationID: "rr-123",
				},
			}
			Expect(ea.Spec.ClusterID).To(Equal("prod-east-1"))
		})

		It("should default to empty string when not set", func() {
			ea := &eav1.EffectivenessAssessment{
				Spec: eav1.EffectivenessAssessmentSpec{
					CorrelationID: "rr-456",
				},
			}
			Expect(ea.Spec.ClusterID).To(BeEmpty())
		})
	})
})
