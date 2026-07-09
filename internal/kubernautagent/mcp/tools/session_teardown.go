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
	"errors"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// CompleteHTTPSession bridges the MCP session lifecycle event to the HTTP session
// store that the AA controller polls.
//
// #1654: duplicate sessions can share the same remediation_id — e.g. an MCP
// action=start fallback session created before AA's own autonomous
// investigation session for the same rrID transitions out of Running. AA
// polls its own session, which may not be the one FindUserDrivingByRemediationID
// finds. Completing only the first match left the sibling session stuck
// non-terminal (its inactivity timer was the only thing that would eventually
// resolve it, minutes later). CompleteHTTPSession therefore always attempts
// BOTH completion paths — CompleteUserDriving for the user_driving match (if
// any) AND ForceCompleteByRemediationID for every other non-terminal sibling
// — so no session sharing the remediation_id is left behind. A critical error
// is logged only when NEITHER path actually completed anything, since
// ErrSessionNotFound from the force-complete path is the expected/common case
// once the user_driving session already resolved every session that existed.
// Exported for use by cmd/kubernautagent/main.go (timeout/disconnect handlers).
func CompleteHTTPSession(completer HTTPSessionCompleter, rrID string, result *katypes.InvestigationResult, logger logr.Logger, action string) {
	if completer == nil {
		return
	}

	var userDrivingCompleted bool
	if httpSessionID, found := completer.FindUserDrivingByRemediationID(rrID); found {
		if err := completer.CompleteUserDriving(httpSessionID, result); err != nil {
			logger.Error(err, "failed to complete user_driving HTTP session",
				"action", action, "rr_id", rrID, "http_session_id", httpSessionID)
		} else {
			userDrivingCompleted = true
		}
	}

	forceCompleteErr := completer.ForceCompleteByRemediationID(rrID, result)
	forceCompleted := forceCompleteErr == nil

	if !userDrivingCompleted && !forceCompleted {
		if errors.Is(forceCompleteErr, session.ErrSessionNotFound) {
			logger.Error(forceCompleteErr, "CRITICAL: no session found for either completion path — AA will not receive result",
				"action", action, "rr_id", rrID)
		} else {
			logger.Error(forceCompleteErr, "CRITICAL: both completion paths failed — AA will not receive result",
				"action", action, "rr_id", rrID)
		}
	}
}
