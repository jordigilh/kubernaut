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

package adapters_test

import (
	"context"
	"testing"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
)

// FuzzPrometheusAdapterParse exercises PrometheusAdapter.Parse (BR-GATEWAY-001..010)
// with adversarial byte input to surface panics or unrecovered errors in the
// AlertManager webhook JSON parsing path. This is a security-relevant fuzz
// target: the Gateway's /api/v1/signals/prometheus route feeds this method
// raw, untrusted bytes directly from the network.
//
// Both adapter dependencies are nil-safe, keeping the target fully offline
// and deterministic: with registry == nil, extractTargetResource short-circuits
// without any Kubernetes API calls; with resolver == nil, fingerprinting falls
// back to a pure SHA256 hash.
//
// NOTE: native Go fuzzing (func FuzzXxx(f *testing.F)) is the sole,
// documented exception to AGENTS.md's Ginkgo/Gomega mandate -- see
// "Exception: Go Native Fuzz Tests" in AGENTS.md. This is a Go toolchain
// constraint (go test -fuzz= only recognizes this exact stdlib signature),
// not a deviation from project convention. This file intentionally
// contains no business-outcome assertions.
//
// Run locally with: go test ./pkg/gateway/adapters/ -run=^$ -fuzz=FuzzPrometheusAdapterParse
func FuzzPrometheusAdapterParse(f *testing.F) {
	seeds := []string{
		`{"alerts": [{"labels": {"alertname": "HighMemoryUsage", "namespace": "production", "pod": "api-server-1"}}]}`,
		`{"alerts": [{"labels": {"alertname": "Test", "namespace": "prod"}, "annotations": {"summary": "test"}}]}`,
		`{"alerts": []}`,
		`{}`,
		`not json`,
		`{"alerts": [{"labels": {}}]}`,
		`{"alerts": [{"labels": {"namespace": "-invalid-", "pod": ""}}]}`,
		`{"alerts": [{"labels": null}]}`,
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}

	adapter := adapters.NewPrometheusAdapter(nil, nil)
	ctx := context.Background()

	f.Fuzz(func(t *testing.T, rawData []byte) {
		// The only contract under test: Parse must never panic, regardless of
		// whether it accepts or rejects the payload.
		_, _ = adapter.Parse(ctx, rawData)
	})
}

// FuzzPrometheusAdapterParseBatch exercises PrometheusAdapter.ParseBatch
// (#1032), the multi-alert batch path with its own per-alert error handling
// distinct from Parse. Same untrusted-input surface and offline construction
// as FuzzPrometheusAdapterParse above.
func FuzzPrometheusAdapterParseBatch(f *testing.F) {
	seeds := []string{
		`{"alerts": [{"labels": {"alertname": "HighMemoryUsage", "namespace": "production", "pod": "api-server-1"}}, {"labels": {"alertname": "HighCPU", "namespace": "production", "pod": "api-server-2"}}]}`,
		`{"alerts": []}`,
		`{}`,
		`not json`,
		`{"alerts": [{"labels": {}}, {"labels": null}]}`,
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}

	adapter := adapters.NewPrometheusAdapter(nil, nil)
	ctx := context.Background()

	f.Fuzz(func(t *testing.T, rawData []byte) {
		_, _ = adapter.ParseBatch(ctx, rawData)
	})
}
