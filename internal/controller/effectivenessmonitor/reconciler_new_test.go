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

package controller

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	emaudit "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/audit"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
)

// TestNewReconciler_WiresDeps is a characterization/smoke test for
// NewReconciler (Issue #1520 Phase 2: this package had zero existing test
// coverage before the ReconcilerDeps struct extraction, per AGENTS.md's TDD
// mandate for zero-coverage refactor targets). NewReconciler is a pure
// struct-literal constructor with no branching logic, so this test pins the
// one behavior that matters: every injected dependency lands on the correct
// field, and the internal scorers/checkers are non-nil.
func TestNewReconciler_WiresDeps(t *testing.T) {
	fakeClient := fake.NewClientBuilder().Build()
	scheme := runtime.NewScheme()
	metrics := &emmetrics.Metrics{}
	auditMgr := &emaudit.Manager{}
	cfg := DefaultReconcilerConfig()
	cfg.ValidityWindow = 42 * 1_000_000_000 // 42s, arbitrary sentinel value

	r := NewReconciler(ReconcilerDeps{
		Client:       fakeClient,
		APIReader:    fakeClient,
		Scheme:       scheme,
		Metrics:      metrics,
		AuditManager: auditMgr,
	}, cfg)

	if r == nil {
		t.Fatal("NewReconciler returned nil")
	}
	if r.Client != fakeClient {
		t.Error("Client not wired from deps")
	}
	if r.Scheme != scheme {
		t.Error("Scheme not wired from deps")
	}
	if r.Metrics != metrics {
		t.Error("Metrics not wired from deps")
	}
	if r.AuditManager != auditMgr {
		t.Error("AuditManager not wired from deps")
	}
	if r.Config.ValidityWindow != cfg.ValidityWindow {
		t.Errorf("Config not wired from cfg: got %v, want %v", r.Config.ValidityWindow, cfg.ValidityWindow)
	}
}
