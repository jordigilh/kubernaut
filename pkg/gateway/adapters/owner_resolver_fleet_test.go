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

package adapters_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet/fleettest"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

var _ = Describe("PrometheusAdapter — Fleet remote owner chain (P1)", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("SetReaderFactory dispatch", func() {
		It("UT-GW-P1-001 [AC-3]: Parse uses local resolver when clusterID is empty", func() {
			localResolver := &stubOwnerResolver{
				ownerKind: "Deployment",
				ownerName: "nginx",
			}
			adapter := adapters.NewPrometheusAdapter(localResolver, nil, logr.Discard())
			adapter.SetReaderFactory(&fleettest.StubReaderFactory{Readers: map[string]client.Reader{}})

			payload := buildAlertPayload("")
			signal, err := adapter.Parse(ctx, payload)
			Expect(err).ToNot(HaveOccurred())
			Expect(signal).ToNot(BeNil())
			Expect(signal.ClusterID).To(BeEmpty(),
				"local signal must not have ClusterID")
		})

		It("UT-GW-P1-002 [AC-3]: Parse constructs remote resolver when clusterID is non-empty", func() {
			localResolver := &stubOwnerResolver{
				ownerKind: "Deployment",
				ownerName: "local-nginx",
			}
			adapter := adapters.NewPrometheusAdapter(localResolver, nil, logr.Discard())
			adapter.SetReaderFactory(&fleettest.StubReaderFactory{
				Readers: map[string]client.Reader{
					"prod-east": nil,
				},
			})

			payload := buildAlertPayload("prod-east")
			signal, err := adapter.Parse(ctx, payload)
			Expect(err).ToNot(HaveOccurred())
			Expect(signal).ToNot(BeNil())
			Expect(signal.ClusterID).To(Equal("prod-east"),
				"remote signal must preserve ClusterID")
		})

		It("UT-GW-P1-003 [AC-3]: Parse falls back to local resolver when readerFactory is nil", func() {
			localResolver := &stubOwnerResolver{
				ownerKind: "Deployment",
				ownerName: "nginx",
			}
			adapter := adapters.NewPrometheusAdapter(localResolver, nil, logr.Discard())

			payload := buildAlertPayload("prod-east")
			signal, err := adapter.Parse(ctx, payload)
			Expect(err).ToNot(HaveOccurred())
			Expect(signal).ToNot(BeNil(),
				"should fall back to local resolver when no readerFactory is set")
		})

		It("UT-GW-P1-004 [AC-3]: Parse uses resource-level fingerprint when readerFactory returns error", func() {
			localResolver := &stubOwnerResolver{
				ownerKind: "Deployment",
				ownerName: "nginx",
			}
			adapter := adapters.NewPrometheusAdapter(localResolver, nil, logr.Discard())
			adapter.SetReaderFactory(&fleettest.StubReaderFactory{
				Err: fmt.Errorf("MCP session unavailable"),
			})

			payload := buildAlertPayload("prod-east")
			signal, err := adapter.Parse(ctx, payload)
			Expect(err).ToNot(HaveOccurred())
			Expect(signal).ToNot(BeNil(),
				"should gracefully degrade when remote resolver construction fails")
		})

		It("UT-GW-P1-005 [AC-3]: SetReaderFactory is optional and does not break existing behavior", func() {
			localResolver := &stubOwnerResolver{
				ownerKind: "Deployment",
				ownerName: "nginx",
			}
			adapter := adapters.NewPrometheusAdapter(localResolver, nil, logr.Discard())

			payload := buildAlertPayload("")
			signal, err := adapter.Parse(ctx, payload)
			Expect(err).ToNot(HaveOccurred())
			Expect(signal).ToNot(BeNil())
			Expect(signal.Resource.Kind).To(Equal("Deployment"))
			Expect(signal.Resource.Name).To(Equal("nginx"))
		})

		It("UT-GW-P1-006 [AU-3]: remote signal includes cluster-aware fingerprint", func() {
			localResolver := &stubOwnerResolver{
				ownerKind: "Deployment",
				ownerName: "nginx",
			}
			adapter := adapters.NewPrometheusAdapter(localResolver, nil, logr.Discard())

			payloadLocal := buildAlertPayload("")
			signalLocal, err := adapter.Parse(ctx, payloadLocal)
			Expect(err).ToNot(HaveOccurred())

			payloadRemote := buildAlertPayload("prod-east")
			signalRemote, err := adapter.Parse(ctx, payloadRemote)
			Expect(err).ToNot(HaveOccurred())

			Expect(signalRemote.Fingerprint).ToNot(Equal(signalLocal.Fingerprint),
				"remote and local signals for same resource should have different fingerprints due to clusterID")
		})

		It("UT-GW-P1-007 [AC-3]: ParseBatch handles remote cluster alerts", func() {
			localResolver := &stubOwnerResolver{
				ownerKind: "Deployment",
				ownerName: "nginx",
			}
			adapter := adapters.NewPrometheusAdapter(localResolver, nil, logr.Discard())

			payload := buildAlertPayload("prod-east")
			signals, err := adapter.ParseBatch(ctx, payload)
			Expect(err).ToNot(HaveOccurred())
			Expect(signals).ToNot(BeEmpty())
			Expect(signals[0].ClusterID).To(Equal("prod-east"),
				"ParseBatch must preserve clusterID from remote alerts")
		})
	})
})

// stubOwnerResolver returns fixed owner resolution results for testing.
type stubOwnerResolver struct {
	ownerKind string
	ownerName string
	err       error
}

func (s *stubOwnerResolver) ResolveTopLevelOwner(_ context.Context, _, _, _ string) (string, string, error) {
	return s.ownerKind, s.ownerName, s.err
}

var _ types.OwnerResolver = (*stubOwnerResolver)(nil)

// buildAlertPayload creates a minimal AlertManager webhook JSON payload for testing.
func buildAlertPayload(clusterID string) []byte {
	commonLabels := map[string]string{}
	if clusterID != "" {
		commonLabels["cluster"] = clusterID
	}

	labels := map[string]string{
		"alertname": "HighCPU",
		"severity":  "warning",
		"namespace": "default",
		"pod":       "nginx-abc",
	}

	payload := map[string]any{
		"status": "firing",
		"alerts": []map[string]any{
			{
				"status": "firing",
				"labels": labels,
			},
		},
		"commonLabels": commonLabels,
	}
	data, _ := json.Marshal(payload)
	return data
}
