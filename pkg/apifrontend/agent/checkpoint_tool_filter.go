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

package agent

import (
	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	adksession "google.golang.org/adk/session"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// checkpointToolFilter is a BeforeModelCallback that removes phase-gated MCP
// tools from the model's visible tool list while their checkpoint flag is
// set (DD-AF-011, #1899). This is the primary enforcement layer: the model
// never sees kubernaut_discover_workflows/kubernaut_select_workflow as an
// option at all while a checkpoint is blocking it, which is a stronger
// control than rejecting the call after the model attempts it (AC-6, SI-10).
//
// phaseGuardBefore (phase_guard.go) is the backstop layer for the rare case
// a call slips past this filter (e.g. a provider quirk or race).
func checkpointToolFilter(ctx agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
	if req == nil || len(req.Tools) == 0 {
		return nil, nil
	}

	state := ctx.State()
	if state == nil {
		return nil, nil
	}

	if checkpointFlagSet(state, session.StateKeyPhase2Blocked) {
		delete(req.Tools, "kubernaut_discover_workflows")
	}
	if checkpointFlagSet(state, session.StateKeyPhase3Blocked) {
		delete(req.Tools, "kubernaut_select_workflow")
	}

	return nil, nil
}

func checkpointFlagSet(state adksession.State, key string) bool {
	v, err := state.Get(key)
	if err != nil {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}
