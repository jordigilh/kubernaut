package agentclient

// v1.5 compatibility: *HTTPError implements session status response interface.
// In main (#963), this was replaced by typed NotFound/InternalServerError
// response types. This file bridges the gap so handler.go compiles against
// both v1.5 ogen schemas and main's ogen schemas (CI PR merge).
// Remove this file when development/v1.5 merges to main and handler.go is
// updated to use the typed responses.
func (*HTTPError) incidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetRes() {}
