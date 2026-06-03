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

package tools

import (
	"github.com/go-logr/logr"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// CompleteHTTPSession bridges the MCP session lifecycle event to the HTTP session
// store that the AA controller polls. It attempts FindUserDriving first, falling
// back to ForceComplete when the session is not in user_driving status.
// Exported for use by cmd/kubernautagent/main.go (timeout/disconnect handlers).
func CompleteHTTPSession(completer HTTPSessionCompleter, rrID string, result *katypes.InvestigationResult, logger logr.Logger, action string) {
	if completer == nil {
		return
	}
	httpSessionID, found := completer.FindUserDrivingByRemediationID(rrID)
	if found {
		if err := completer.CompleteUserDriving(httpSessionID, result); err != nil {
			logger.Error(err, "failed to complete HTTP session",
				"action", action, "rr_id", rrID, "http_session_id", httpSessionID)
		}
	} else if err := completer.ForceCompleteByRemediationID(rrID, result); err != nil {
		logger.V(1).Info("no HTTP session found to force-complete",
			"action", action, "rr_id", rrID, "error", err)
	}
}
