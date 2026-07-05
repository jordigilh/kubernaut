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

package gateway_test

import (
	"context"
	"testing"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
)

// FuzzKubernetesEventAdapterParse exercises KubernetesEventAdapter.Parse
// (BR-GATEWAY-005) with adversarial byte input to surface panics or
// unrecovered errors in the Kubernetes Event API JSON parsing path. This is
// a security-relevant fuzz target: the Gateway's
// /api/v1/signals/kubernetes-event route feeds this method raw, untrusted
// bytes directly from the network.
//
// Constructed with no owner resolver, keeping the target fully offline and
// deterministic: fingerprinting falls back to a pure SHA256 hash.
//
// NOTE: native Go fuzzing (func FuzzXxx(f *testing.F)) is the sole,
// documented exception to AGENTS.md's Ginkgo/Gomega mandate -- see
// "Exception: Go Native Fuzz Tests" in AGENTS.md. This is a Go toolchain
// constraint (go test -fuzz= only recognizes this exact stdlib signature),
// not a deviation from project convention. This file intentionally
// contains no business-outcome assertions.
//
// Run locally with: go test ./pkg/gateway/ -run=^$ -fuzz=FuzzKubernetesEventAdapterParse
func FuzzKubernetesEventAdapterParse(f *testing.F) {
	seeds := []string{
		`{"type": "Warning", "reason": "OOMKilled", "message": "Container killed due to memory limit", "involvedObject": {"kind": "Pod", "namespace": "production", "name": "payment-api-789"}}`,
		`{"type": "Normal", "reason": "Scheduled"}`,
		`{}`,
		`not json`,
		`{"involvedObject": null}`,
		`{"type": "Warning", "involvedObject": {"kind": "", "namespace": "", "name": ""}}`,
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}

	adapter := adapters.NewKubernetesEventAdapter()
	ctx := context.Background()

	f.Fuzz(func(t *testing.T, rawData []byte) {
		// The only contract under test: Parse must never panic, regardless of
		// whether it accepts or rejects the payload.
		_, _ = adapter.Parse(ctx, rawData)
	})
}
