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

package main

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	auth "github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// TestBuildMCPHandler_Guards is a characterization test for buildMCPHandler's
// three required-dependency guard clauses (Issue #1520 Phase 2: pins behavior
// before the mcpHandlerParams struct extraction, per AGENTS.md's TDD mandate
// for zero-coverage refactor targets). Each case returns before touching the
// network (ctrlclient.New), so no live cluster is needed.
func TestBuildMCPHandler_Guards(t *testing.T) {
	validInfra := &k8sInfra{kubeConfig: &rest.Config{}}
	validAuthMw := &auth.Middleware{}
	validInv := &investigator.Investigator{}

	tests := []struct {
		name   string
		params mcpHandlerParams
	}{
		{
			name:   "nil infra",
			params: mcpHandlerParams{infra: nil, authMw: validAuthMw, inv: validInv, logger: logr.Discard()},
		},
		{
			name:   "infra with nil kubeConfig",
			params: mcpHandlerParams{infra: &k8sInfra{}, authMw: validAuthMw, inv: validInv, logger: logr.Discard()},
		},
		{
			name:   "nil auth middleware (DD-AUTH-MCP-001)",
			params: mcpHandlerParams{infra: validInfra, authMw: nil, inv: validInv, logger: logr.Discard()},
		},
		{
			name:   "nil investigator (SEC-05)",
			params: mcpHandlerParams{infra: validInfra, authMw: validAuthMw, inv: nil, logger: logr.Discard()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, drainer := buildMCPHandler(context.Background(), tt.params)
			if handler != nil {
				t.Errorf("expected nil handler, got %v", handler)
			}
			if drainer != nil {
				t.Errorf("expected nil session drainer, got %v", drainer)
			}
		})
	}
}
