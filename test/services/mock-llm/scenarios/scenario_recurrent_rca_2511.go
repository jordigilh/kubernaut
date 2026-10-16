/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package scenarios

import (
	"strings"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/pkg/shared/uuid"
)

// recurrentRCA2511Scenario is intentionally remediation-type agnostic. The
// first response identifies one target; a server-injected history review causes
// the next response to identify a different target and classify the mechanism.
type recurrentRCA2511ScenarioImpl struct {
	*configScenario
}

func recurrentRCA2511Scenario() Scenario {
	rcaConfidence := 0.91
	cfg := MockScenarioConfig{
		ScenarioName:          "recurrent_rca_2511",
		SignalName:            "recurrentrca2511",
		Severity:              "high",
		WorkflowName:          "generic-recurrence-remediation",
		WorkflowID:            uuid.DeterministicUUID("generic-recurrence-remediation"),
		ActionType:            "generic_remediation",
		Confidence:            0.84,
		RCAConfidence:         &rcaConfidence,
		RootCause:             "Current target evidence is insufficient to verify the deeper mechanism",
		ResourceKind:          "Deployment",
		ResourceNS:            "prod",
		ResourceName:          "api-a",
		APIVersion:            "apps/v1",
		ToolCallName:          openai.ToolSubmitResult,
		CausalMechanismStatus: "unknown",
	}
	base := newSelectorScenario("recurrent_rca_2511", ScenarioSelector{
		Scope: ScenarioScope{Caller: CallerKA}, SignalPatterns: []string{cfg.SignalName}, Confidence: 1.0,
	}, cfg)
	return &recurrentRCA2511ScenarioImpl{configScenario: base}
}

func (s *recurrentRCA2511ScenarioImpl) ConfigForContext(ctx *DetectionContext) MockScenarioConfig {
	cfg := s.config
	cfg.ToolCallName = openai.ToolSubmitResult
	if ctx != nil && strings.Contains(strings.ToLower(ctx.AllText), "prior remediation history review") {
		cfg.ResourceName = "api-b"
		cfg.RootCause = "The prior outcome is evidence but does not verify the current causal mechanism"
		cfg.CausalMechanismStatus = "probable"
	}
	return cfg
}
