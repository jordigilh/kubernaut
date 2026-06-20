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
	"testing"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

func setupPhaseResolver(t *testing.T) (*llm.SwappableClient, *investigator.DefaultPhaseResolver) {
	t.Helper()
	inner := &stubLLMClient{}
	sc, err := llm.NewSwappableClient(inner, "default-model")
	if err != nil {
		t.Fatal(err)
	}
	resolver := investigator.NewDefaultPhaseResolver(sc, nil)
	return sc, resolver
}

// IT-AI-1470-004a (CM-3): Hot-reload with phaseModels rebuild
func TestReloadAI1470_004a_PhaseModelsRebuild(t *testing.T) {
	sc, resolver := setupPhaseResolver(t)

	cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), resolver)

	err := cb(`model: gpt-4
endpoint: http://localhost:11434
apiKey: test-key
phaseModels:
  workflow_discovery:
    model: gpt-4-mini
    endpoint: http://fast-endpoint:11434
`)
	if err != nil {
		t.Fatalf("reload with phaseModels should succeed, got: %v", err)
	}

	_, wfModel, _ := resolver.ResolvePhase(katypes.PhaseWorkflowDiscovery)
	if wfModel != "gpt-4-mini" {
		t.Fatalf("expected workflow_discovery model gpt-4-mini, got %s", wfModel)
	}

	_, rcaModel, _ := resolver.ResolvePhase(katypes.PhaseRCA)
	if rcaModel != "gpt-4" {
		t.Fatalf("expected RCA to use default (reloaded) model gpt-4, got %s", rcaModel)
	}
}

// IT-AI-1470-004b: Hot-reload adding a new phase override
func TestReloadAI1470_004b_AddPhaseOverride(t *testing.T) {
	sc, resolver := setupPhaseResolver(t)

	_, rcaModelBefore, _ := resolver.ResolvePhase(katypes.PhaseRCA)
	if rcaModelBefore != "default-model" {
		t.Fatalf("before reload, expected default model, got %s", rcaModelBefore)
	}

	cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), resolver)

	err := cb(`model: gpt-4
endpoint: http://localhost:11434
apiKey: test-key
phaseModels:
  rca:
    model: claude-sonnet
    endpoint: http://anthropic:443
`)
	if err != nil {
		t.Fatalf("reload adding rca override should succeed, got: %v", err)
	}

	_, rcaModel, _ := resolver.ResolvePhase(katypes.PhaseRCA)
	if rcaModel != "claude-sonnet" {
		t.Fatalf("expected rca model claude-sonnet, got %s", rcaModel)
	}
}

// IT-AI-1470-004c: Hot-reload removing a phase override
func TestReloadAI1470_004c_RemovePhaseOverride(t *testing.T) {
	sc, resolver := setupPhaseResolver(t)

	wfInner := &stubLLMClient{}
	wfSw, err := llm.NewSwappableClient(wfInner, "fast-model")
	if err != nil {
		t.Fatal(err)
	}
	resolver.SetPhaseSwappable(katypes.PhaseWorkflowDiscovery, wfSw)

	_, wfModelBefore, _ := resolver.ResolvePhase(katypes.PhaseWorkflowDiscovery)
	if wfModelBefore != "fast-model" {
		t.Fatalf("before reload, expected fast-model, got %s", wfModelBefore)
	}

	cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), resolver)

	err = cb(`model: gpt-4
endpoint: http://localhost:11434
apiKey: test-key
`)
	if err != nil {
		t.Fatalf("reload without phaseModels should succeed, got: %v", err)
	}

	_, wfModelAfter, _ := resolver.ResolvePhase(katypes.PhaseWorkflowDiscovery)
	if wfModelAfter != "gpt-4" {
		t.Fatalf("after removing phase override, expected default (reloaded) model gpt-4, got %s", wfModelAfter)
	}
}

// Backward compat: nil resolver does not break existing reload
func TestReloadAI1470_NilResolverBackwardCompat(t *testing.T) {
	sc := setupSwappable(t)
	cb := llmRuntimeReloadCallback(staticCfg(), sc, testReloadLogger(), nil)

	err := cb(`model: gpt-4-turbo
endpoint: http://localhost:11434
apiKey: test-key
`)
	if err != nil {
		t.Fatalf("reload with nil resolver should succeed, got: %v", err)
	}
	if sc.ModelName() != "gpt-4-turbo" {
		t.Fatalf("expected model gpt-4-turbo, got %s", sc.ModelName())
	}
}
