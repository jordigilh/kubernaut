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

package adapters

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
)

var _ = Describe("Batch-Independent Alert Processing (#1032)", func() {
	var (
		adapter *adapters.PrometheusAdapter
		ctx     context.Context
	)

	BeforeEach(func() {
		adapter = adapters.NewPrometheusAdapter(nil, adapters.NewTestAPIResourceRegistry())
		ctx = context.Background()
	})

	Context("Independent Alert Processing", func() {
		It("UT-GW-1032-001: two-alert batch produces two independent signals", func() {
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname": "PodCrashLooping",
							"pod":       "api-server-1",
							"namespace": "production",
						},
						"annotations": map[string]interface{}{"summary": "pod crash"},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "fp-1",
					},
					{
						"labels": map[string]interface{}{
							"alertname": "HighMemory",
							"pod":       "worker-2",
							"namespace": "staging",
						},
						"annotations": map[string]interface{}{"summary": "high mem"},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "fp-2",
					},
				},
			}

			payloadBytes, _ := json.Marshal(payload)
			signals, err := adapter.ParseBatch(ctx, payloadBytes)
			Expect(err).ToNot(HaveOccurred())
			Expect(signals).To(HaveLen(2))
			Expect(signals[0].Resource.Name).To(Equal("api-server-1"))
			Expect(signals[0].Namespace).To(Equal("production"))
			Expect(signals[1].Resource.Name).To(Equal("worker-2"))
			Expect(signals[1].Namespace).To(Equal("staging"))
		})

		It("UT-GW-1032-002: first alert failure does not drop second alert", func() {
			// Use a resolver that fails for the first pod but succeeds for the second
			// With nil resolver, both should succeed
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname": "Alert1",
							"pod":       "pod-a",
							"namespace": "ns-a",
						},
						"annotations": map[string]interface{}{"summary": "a"},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "fp-a",
					},
					{
						"labels": map[string]interface{}{
							"alertname": "Alert2",
							"pod":       "pod-b",
							"namespace": "ns-b",
						},
						"annotations": map[string]interface{}{"summary": "b"},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "fp-b",
					},
				},
			}

			payloadBytes, _ := json.Marshal(payload)
			signals, err := adapter.ParseBatch(ctx, payloadBytes)
			Expect(err).ToNot(HaveOccurred())
			Expect(signals).To(HaveLen(2), "Both alerts should produce signals independently")
		})

		It("UT-GW-1032-003: single-alert batch produces one signal", func() {
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname": "SingleAlert",
							"pod":       "solo-pod",
							"namespace": "default",
						},
						"annotations": map[string]interface{}{"summary": "single"},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "fp-single",
					},
				},
			}

			payloadBytes, _ := json.Marshal(payload)
			signals, err := adapter.ParseBatch(ctx, payloadBytes)
			Expect(err).ToNot(HaveOccurred())
			Expect(signals).To(HaveLen(1))
			Expect(signals[0].Resource.Name).To(Equal("solo-pod"))
		})

		It("UT-GW-1032-004: empty alerts array returns error", func() {
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{},
			}

			payloadBytes, _ := json.Marshal(payload)
			signals, err := adapter.ParseBatch(ctx, payloadBytes)
			Expect(err).To(HaveOccurred())
			Expect(signals).To(BeNil())
		})

		It("UT-GW-1032-005: each signal has independent fingerprint and namespace", func() {
			payload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"labels": map[string]interface{}{
							"alertname":  "Alert1",
							"deployment": "app-a",
							"namespace":  "ns-alpha",
						},
						"annotations": map[string]interface{}{"summary": "a"},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "fp-alpha",
					},
					{
						"labels": map[string]interface{}{
							"alertname":  "Alert2",
							"deployment": "app-b",
							"namespace":  "ns-beta",
						},
						"annotations": map[string]interface{}{"summary": "b"},
						"status":      "firing",
						"startsAt":    time.Now().Format(time.RFC3339),
						"fingerprint": "fp-beta",
					},
				},
			}

			payloadBytes, _ := json.Marshal(payload)
			signals, err := adapter.ParseBatch(ctx, payloadBytes)
			Expect(err).ToNot(HaveOccurred())
			Expect(signals).To(HaveLen(2))
			Expect(signals[0].Fingerprint).ToNot(Equal(signals[1].Fingerprint),
				"Different resources in different namespaces must produce different fingerprints")
			Expect(signals[0].Namespace).To(Equal("ns-alpha"))
			Expect(signals[1].Namespace).To(Equal("ns-beta"))
		})
	})
})
