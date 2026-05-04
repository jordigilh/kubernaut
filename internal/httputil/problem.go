package httputil

import (
	"encoding/json"
	"net/http"
)

// ProblemDetail represents an RFC 7807 Problem Details response.
type ProblemDetail struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// WriteProblem writes an RFC 7807 application/problem+json response.
func WriteProblem(w http.ResponseWriter, status int, title, detail string) {
	problem := ProblemDetail{
		Type:   "about:blank",
		Title:  title,
		Status: status,
		Detail: detail,
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem)
}

// WriteProblemWithType writes an RFC 7807 response with a custom type URI.
func WriteProblemWithType(w http.ResponseWriter, status int, typeURI, title, detail, instance string) {
	problem := ProblemDetail{
		Type:     typeURI,
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: instance,
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem)
}
