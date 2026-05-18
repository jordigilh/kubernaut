package session_test

import (
	adksession "google.golang.org/adk/session"
)

func createRequestWithDefaults(sessionID, userID string, state map[string]any) adksession.CreateRequest {
	return adksession.CreateRequest{
		AppName:   "kubernaut-apifrontend",
		UserID:    userID,
		SessionID: sessionID,
		State:     state,
	}
}
