package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
)

const (
	backendContractPath         = "/.well-known/kubernaut-console-contract.json"
	backendIncarnationHeader    = "X-Kubernaut-Backend-Incarnation"
	backendContractContentType  = "application/json"
	backendContractCacheControl = "no-store"
)

var canonicalBackendIncarnation = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type backendContractResponse struct {
	BackendIncarnation string `json:"backend_incarnation"`
}

// validateBackendIncarnation accepts only the bounded, lowercase UUIDv4 wire
// representation used by the public Console contract and response header.
func validateBackendIncarnation(value string) error {
	if len(value) != 36 || !canonicalBackendIncarnation.MatchString(value) {
		return fmt.Errorf("backend incarnation must be a canonical lowercase uuidv4")
	}
	return nil
}

// ValidateBackendIncarnation validates a value before it is placed in the
// public contract or an HTTP response header.
func ValidateBackendIncarnation(value string) error {
	return validateBackendIncarnation(value)
}

func writeBackendContract(w http.ResponseWriter, incarnation string) {
	w.Header().Set("Content-Type", backendContractContentType)
	w.Header().Set("Cache-Control", backendContractCacheControl)
	payload, _ := json.Marshal(backendContractResponse{BackendIncarnation: incarnation})
	_, _ = w.Write(payload)
}
