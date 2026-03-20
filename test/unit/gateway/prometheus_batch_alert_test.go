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

package gateway

import (
	"context"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// batchAlertEntry defines a single alert for multi-alert webhook construction.
type batchAlertEntry struct {
	Alertname   string
	Severity    string
	Namespace   string
	Pod         string
	Annotations map[string]string
}

// newBatchWebhookJSON builds an AlertManager webhook payload with multiple alerts.
func newBatchWebhookJSON(alerts []batchAlertEntry, commonLabels map[string]string) []byte {
	type alertJSON struct {
		Status      string            `json:"status"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
		StartsAt    string            `json:"startsAt"`
	}
	type webhookJSON struct {
		Alerts           []alertJSON       `json:"alerts"`
		CommonLabels     map[string]string `json:"commonLabels"`
		CommonAnnotations map[string]string `json:"commonAnnotations"`
	}

	var items []alertJSON
	for _, a := range alerts {
		labels := map[string]string{
			"alertname": a.Alertname,
			"namespace": a.Namespace,
			"severity":  a.Severity,
			"pod":       a.Pod,
		}
		ann := a.Annotations
		if ann == nil {
			ann = map[string]string{"summary": "test alert"}
		}
		items = append(items, alertJSON{
			Status:      "firing",
			Labels:      labels,
			Annotations: ann,
			StartsAt:    "2026-03-20T10:00:00Z",
		})
	}

	if commonLabels == nil {
		commonLabels = map[string]string{}
	}
	payload, _ := json.Marshal(webhookJSON{
		Alerts:            items,
		CommonLabels:      commonLabels,
		CommonAnnotations: map[string]string{},
	})
	return payload
}

var _ = Describe("Issue #451: Gateway Resilient Batch Alert Processing", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	// Resolver that fails for specific "stale" pods and succeeds for others.
	// stalePods is a set of pod names that should fail resolution.
	newSelectiveResolver := func(stalePods map[string]bool) *mockOwnerResolver {
		return &mockOwnerResolver{
			resolveFunc: func(ctx context.Context, namespace, kind, name string) (string, string, error) {
				if stalePods[name] {
					return "", "", fmt.Errorf("pods %q not found", name)
				}
				return "Deployment", "worker", nil
			},
		}
	}

	Describe("Batch with stale and valid alerts", func() {
		It("UT-GW-451-001: should process valid alert when stale alert appears first", func() {
			stalePod := "worker-5b6cc47c55-6bq7q"
			validPod := "worker-5b6cc47c55-kfmg9"

			resolver := newSelectiveResolver(map[string]bool{stalePod: true})
			adapter := adapters.NewPrometheusAdapter(resolver, nil)

			payload := newBatchWebhookJSON([]batchAlertEntry{
				{Alertname: "KubePodCrashLooping", Severity: "critical", Namespace: "demo-crashloop", Pod: stalePod},
				{Alertname: "KubePodCrashLooping", Severity: "critical", Namespace: "demo-crashloop", Pod: validPod},
			}, nil)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).ToNot(HaveOccurred(),
				"Batch must not fail when at least one alert resolves successfully")
			Expect(signal.SignalName).To(Equal("KubePodCrashLooping"))

			expectedFP := types.CalculateOwnerFingerprint(types.ResourceIdentifier{
				Namespace: "demo-crashloop",
				Kind:      "Deployment",
				Name:      "worker",
			})
			Expect(signal.Fingerprint).To(Equal(expectedFP),
				"Signal fingerprint must come from the valid alert's resolved owner")
		})

		It("UT-GW-451-002: should use first valid alert when stale alert is in the middle", func() {
			stalePod := "worker-stale-abc"
			validPod1 := "worker-valid-111"
			validPod2 := "worker-valid-222"

			resolver := newSelectiveResolver(map[string]bool{stalePod: true})
			adapter := adapters.NewPrometheusAdapter(resolver, nil)

			payload := newBatchWebhookJSON([]batchAlertEntry{
				{Alertname: "FirstValidAlert", Severity: "warning", Namespace: "ns1", Pod: validPod1},
				{Alertname: "StaleAlert", Severity: "critical", Namespace: "ns1", Pod: stalePod},
				{Alertname: "SecondValidAlert", Severity: "critical", Namespace: "ns1", Pod: validPod2},
			}, nil)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).ToNot(HaveOccurred())
			Expect(signal.SignalName).To(Equal("FirstValidAlert"),
				"First valid alert in batch order must be selected")
		})
	})

	Describe("All alerts stale", func() {
		It("UT-GW-451-003: should return error when ALL alerts reference deleted pods", func() {
			stalePod1 := "worker-gone-aaa"
			stalePod2 := "worker-gone-bbb"

			resolver := newSelectiveResolver(map[string]bool{stalePod1: true, stalePod2: true})
			adapter := adapters.NewPrometheusAdapter(resolver, nil)

			payload := newBatchWebhookJSON([]batchAlertEntry{
				{Alertname: "Alert1", Severity: "critical", Namespace: "ns1", Pod: stalePod1},
				{Alertname: "Alert2", Severity: "warning", Namespace: "ns1", Pod: stalePod2},
			}, nil)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).To(HaveOccurred(),
				"Batch must fail when no alert resolves successfully")
			Expect(signal).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("all"),
				"Error should indicate that all alerts in the batch failed")
		})
	})

	Describe("No regression for single-alert webhooks", func() {
		It("UT-GW-451-004: should process single valid alert identically to pre-fix behavior", func() {
			resolver := newSelectiveResolver(map[string]bool{})
			adapter := adapters.NewPrometheusAdapter(resolver, nil)

			payload := newBatchWebhookJSON([]batchAlertEntry{
				{Alertname: "KubePodCrashLooping", Severity: "critical", Namespace: "prod", Pod: "api-789"},
			}, nil)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).ToNot(HaveOccurred())

			expectedFP := types.CalculateOwnerFingerprint(types.ResourceIdentifier{
				Namespace: "prod",
				Kind:      "Deployment",
				Name:      "worker",
			})
			Expect(signal.Fingerprint).To(Equal(expectedFP))
			Expect(signal.SignalName).To(Equal("KubePodCrashLooping"))
			Expect(signal.Severity).To(Equal("critical"))
			Expect(signal.Namespace).To(Equal("prod"))
		})

		It("UT-GW-451-005: should return error for single stale alert (existing behavior)", func() {
			stalePod := "deleted-pod-xyz"

			resolver := newSelectiveResolver(map[string]bool{stalePod: true})
			adapter := adapters.NewPrometheusAdapter(resolver, nil)

			payload := newBatchWebhookJSON([]batchAlertEntry{
				{Alertname: "KubePodCrashLooping", Severity: "critical", Namespace: "prod", Pod: stalePod},
			}, nil)

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).To(HaveOccurred(),
				"Single stale alert must still return error")
			Expect(signal).To(BeNil())
		})
	})

	Describe("Signal field correctness from first valid alert", func() {
		It("UT-GW-451-007: should use labels, severity, and alertname from first valid alert, not stale alert", func() {
			stalePod := "stale-pod-000"
			validPod := "valid-pod-111"

			resolver := newSelectiveResolver(map[string]bool{stalePod: true})
			adapter := adapters.NewPrometheusAdapter(resolver, nil)

			payload := newBatchWebhookJSON([]batchAlertEntry{
				{Alertname: "StaleAlert", Severity: "warning", Namespace: "ns-stale", Pod: stalePod,
					Annotations: map[string]string{"summary": "stale summary"}},
				{Alertname: "KubePodCrashLooping", Severity: "critical", Namespace: "demo-crashloop", Pod: validPod,
					Annotations: map[string]string{"summary": "valid summary"}},
			}, map[string]string{"cluster": "test-cluster"})

			signal, err := adapter.Parse(ctx, payload)
			Expect(err).ToNot(HaveOccurred())

			Expect(signal.SignalName).To(Equal("KubePodCrashLooping"),
				"SignalName must come from the first valid alert, not the skipped stale one")
			Expect(signal.Severity).To(Equal("critical"),
				"Severity must come from the first valid alert")
			Expect(signal.Namespace).To(Equal("demo-crashloop"),
				"Namespace must come from the first valid alert")
			Expect(signal.Labels).To(HaveKeyWithValue("cluster", "test-cluster"),
				"CommonLabels must be merged with the valid alert's labels")
			Expect(signal.Annotations).To(HaveKeyWithValue("summary", "valid summary"),
				"Annotations must come from the first valid alert")
		})
	})
})
