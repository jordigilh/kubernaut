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

// Package observation contains the dependency-neutral FMC source observation
// contract shared by the MCP reader and the metadata-cache synchronizer.
package observation

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

const (
	// ObservationPageLimit is the fixed bounded page size for an FMC source scan.
	ObservationPageLimit = 50
	MaxObservationPages  = 100
	MaxObservationItems  = 5000
)

// ListRequest is the exact request shape used by the FMC source observation
// path. A nil Continue or LabelSelector omits that JSON field entirely.
type ListRequest struct {
	APIVersion    string
	Kind          string
	Namespace     string
	Limit         int
	Continue      *string
	LabelSelector *string
}

// ContinuationField preserves whether a response omitted continue or returned
// JSON null. Both are final pages, but preserving the distinction prevents a
// malformed empty string from being silently treated as absent at a boundary.
type ContinuationField struct {
	Present bool
	Null    bool
	Value   string
}

// ObservationFailure identifies why a source observation cannot authorize a
// complete cache cycle.
type ObservationFailure string

const (
	FailureNone             ObservationFailure = ""
	FailureTool             ObservationFailure = "tool_error"
	FailureTransport        ObservationFailure = "transport_error"
	FailureMalformedContent ObservationFailure = "malformed_content"
	FailureContinuation     ObservationFailure = "invalid_continuation"
	FailureBounds           ObservationFailure = "bounds_exceeded"
	FailureItem             ObservationFailure = "invalid_item"
	FailureScope            ObservationFailure = "invalid_scope"
	FailureDuplicate        ObservationFailure = "duplicate_identity"
	FailureConflict         ObservationFailure = "conflicting_identity"
)

// ObservationError carries a typed source failure while retaining the root
// error for logs and tests. Every incomplete observation returns one of these.
type ObservationError struct {
	Failure ObservationFailure
	Err     error
}

func (e *ObservationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return string(e.Failure)
	}
	return fmt.Sprintf("observation %s: %v", e.Failure, e.Err)
}

func (e *ObservationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// FailureOf returns the typed failure from an observation error, if present.
func FailureOf(err error) ObservationFailure {
	if err == nil {
		return FailureNone
	}
	var observationErr *ObservationError
	if errors.As(err, &observationErr) {
		return observationErr.Failure
	}
	return FailureNone
}

// ObservedItem is a validated Kubernetes object plus the bounded managed-label
// state needed by FMC. LabelsPresent distinguishes an absent labels field from
// an explicitly present empty object without storing arbitrary labels.
type ObservedItem struct {
	Object        unstructured.Unstructured
	LabelsPresent bool
	LabelState    scope.LabelState
}

// ListObservation is the complete-scan result consumed by FMC. Complete is
// true only after the final page and every item in every page has validated.
type ListObservation struct {
	Items        []ObservedItem
	Complete     bool
	PageCount    int
	ItemCount    int
	Continuation ContinuationField
	Failure      ObservationFailure
}

// ObservedReader is the strict source-reading seam used by FMC. It is not a
// client.Reader compatibility interface: a bare object list cannot establish
// source completeness or preserve explicit false/invalid label state.
type ObservedReader interface {
	ListObserved(context.Context, ListRequest) (ListObservation, error)
}
