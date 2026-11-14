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

package mcpclient

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/fleet/observation"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// These aliases keep the public MCP package ergonomic while the contract lives
// in the dependency-neutral observation package shared with FMC.
type ListRequest = observation.ListRequest
type ContinuationField = observation.ContinuationField
type ObservationFailure = observation.ObservationFailure
type ObservationError = observation.ObservationError
type ObservedItem = observation.ObservedItem
type ListObservation = observation.ListObservation
type ObservedReader = observation.ObservedReader
type ObservedListReader = observation.ObservedReader

const (
	ObservationPageLimit    = observation.ObservationPageLimit
	maxObservationPages     = observation.MaxObservationPages
	maxObservationItems     = observation.MaxObservationItems
	FailureNone             = observation.FailureNone
	FailureTool             = observation.FailureTool
	FailureTransport        = observation.FailureTransport
	FailureMalformedContent = observation.FailureMalformedContent
	FailureContinuation     = observation.FailureContinuation
	FailureBounds           = observation.FailureBounds
	FailureItem             = observation.FailureItem
	FailureScope            = observation.FailureScope
	FailureDuplicate        = observation.FailureDuplicate
	FailureConflict         = observation.FailureConflict
)

func FailureOf(err error) ObservationFailure {
	return observation.FailureOf(err)
}

func (c *Client) listResources(ctx context.Context, request ListRequest) (ListObservation, error) {
	return c.listResourcesWithValidation(ctx, request, true)
}

func (c *Client) listResourcesForClient(ctx context.Context, request ListRequest) (ListObservation, error) {
	return c.listResourcesWithValidation(ctx, request, false)
}

func (c *Client) listResourcesWithValidation(ctx context.Context, request ListRequest, strict bool) (ListObservation, error) {
	if err := validateListRequest(request, strict); err != nil {
		return failedObservation(FailureScope, err)
	}

	observation := ListObservation{Items: make([]ObservedItem, 0)}
	seen := make(map[string]scope.LabelState)
	continueToken := request.Continue
	for page := 1; page <= maxObservationPages; page++ {
		pageRequest := request
		pageRequest.Continue = continueToken
		result, err := c.callTool(ctx, c.resolveToolName(ToolList), listRequestArguments(pageRequest))
		if err != nil {
			return failedObservation(FailureTransport, errWithPage("list resources", page, err))
		}
		if result == nil {
			return failedObservation(FailureTool, fmt.Errorf("list resources page %d returned no result", page))
		}
		if result.IsError {
			return failedObservation(FailureTool, fmt.Errorf("list resources page %d returned error: %s", page, ExtractText(result)))
		}

		pageItems, continuation, err := extractObservationPage(result, request, strict)
		if err != nil {
			return failedObservation(FailureOf(err), err)
		}
		observation.PageCount++
		observation.Continuation = continuation
		if err := appendObservationItems(&observation, seen, pageItems, page); err != nil {
			return failedObservation(FailureOf(err), err)
		}
		if !continuation.Present || continuation.Null {
			observation.Complete = true
			observation.Failure = FailureNone
			return observation, nil
		}
		if continuation.Value == "" {
			return failedObservation(FailureContinuation, fmt.Errorf("page %d returned an empty continuation token", page))
		}
		if continueToken != nil && continuation.Value == *continueToken {
			return failedObservation(FailureContinuation, fmt.Errorf("page %d repeated continuation token %q", page, continuation.Value))
		}
		next := continuation.Value
		continueToken = &next
	}

	return failedObservation(FailureBounds, fmt.Errorf("observation exceeds %d pages", maxObservationPages))
}

func validateListRequest(request ListRequest, strict bool) error {
	if request.APIVersion == "" || request.Kind == "" {
		return fmt.Errorf("apiVersion and kind are required")
	}
	if request.Limit != ObservationPageLimit {
		return fmt.Errorf("limit must be %d", ObservationPageLimit)
	}
	if request.Continue != nil && *request.Continue == "" {
		return fmt.Errorf("continue must be non-empty when present")
	}
	gv, err := schema.ParseGroupVersion(request.APIVersion)
	if err != nil || gv.Version == "" {
		return fmt.Errorf("invalid apiVersion %q", request.APIVersion)
	}
	if strict {
		if _, ok := observedScope(gv, request.Kind); !ok {
			return fmt.Errorf("unsupported observed GVK %s, Kind=%s", request.APIVersion, request.Kind)
		}
		if isClusterScopedGVK(gv, request.Kind) && request.Namespace != "" {
			return fmt.Errorf("cluster-scoped kind %s must not have a namespace", request.Kind)
		}
	}
	return nil
}

func listRequestArguments(request ListRequest) map[string]any {
	args := map[string]any{
		"kind":       request.Kind,
		"apiVersion": request.APIVersion,
		"limit":      request.Limit,
	}
	if request.Namespace != "" {
		args["namespace"] = request.Namespace
	}
	if request.Continue != nil {
		args["continue"] = *request.Continue
	}
	if request.LabelSelector != nil {
		args["labelSelector"] = *request.LabelSelector
	}
	return args
}

func appendObservationItems(observation *ListObservation, seen map[string]scope.LabelState, pageItems []ObservedItem, page int) error {
	if len(pageItems)+observation.ItemCount > maxObservationItems {
		return observationFailure(FailureBounds, fmt.Errorf("observation exceeds %d items", maxObservationItems))
	}
	for index, item := range pageItems {
		identity := observedIdentity(item.Object)
		if previous, exists := seen[identity]; exists {
			if previous != item.LabelState {
				return observationFailure(FailureConflict, fmt.Errorf("item %q has conflicting label state on page %d item %d", identity, page, index))
			}
			return observationFailure(FailureDuplicate, fmt.Errorf("item %q is repeated on page %d item %d", identity, page, index))
		}
		seen[identity] = item.LabelState
		observation.Items = append(observation.Items, item)
		observation.ItemCount++
	}
	return nil
}

func extractObservationPage(result *mcp.CallToolResult, request ListRequest, strict bool) ([]ObservedItem, ContinuationField, error) {
	if result.StructuredContent == nil {
		return nil, ContinuationField{}, observationFailure(FailureMalformedContent, fmt.Errorf("structuredContent is required"))
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		return nil, ContinuationField{}, observationFailure(FailureMalformedContent, fmt.Errorf("structuredContent must be an object"))
	}
	rawItems, ok := structured["items"]
	if !ok {
		return nil, ContinuationField{}, observationFailure(FailureMalformedContent, fmt.Errorf("structuredContent.items is required"))
	}
	items, ok := rawItems.([]any)
	if !ok {
		if typed, typedOK := rawItems.([]map[string]any); typedOK {
			items = make([]any, 0, len(typed))
			for _, item := range typed {
				items = append(items, item)
			}
		} else {
			return nil, ContinuationField{}, observationFailure(FailureMalformedContent, fmt.Errorf("structuredContent.items must be an array"))
		}
	}

	continuation, err := parseContinuation(structured)
	if err != nil {
		return nil, ContinuationField{}, observationFailure(FailureContinuation, err)
	}
	observed := make([]ObservedItem, 0, len(items))
	for index, rawItem := range items {
		item, err := validateObservedItem(rawItem, request, strict)
		if err != nil {
			return nil, continuation, err
		}
		observed = append(observed, item)
		if index >= maxObservationItems {
			return nil, continuation, observationFailure(FailureBounds, fmt.Errorf("page item bound exceeded"))
		}
	}
	return observed, continuation, nil
}

func parseContinuation(structured map[string]any) (ContinuationField, error) {
	raw, present := structured["continue"]
	if !present {
		return ContinuationField{}, nil
	}
	if raw == nil {
		return ContinuationField{Present: true, Null: true}, nil
	}
	value, ok := raw.(string)
	if !ok || value == "" {
		return ContinuationField{}, fmt.Errorf("structuredContent.continue must be null or a non-empty string")
	}
	return ContinuationField{Present: true, Value: value}, nil
}

func validateObservedItem(rawItem any, request ListRequest, strict bool) (ObservedItem, error) {
	item, ok := rawItem.(map[string]any)
	if !ok {
		return ObservedItem{}, observationFailure(FailureItem, fmt.Errorf("item is not an object"))
	}
	if err := validateObservedGVK(item, request); err != nil {
		return ObservedItem{}, observationFailure(FailureItem, err)
	}
	metadata, name, namespace, err := observedMetadata(item)
	if err != nil {
		return ObservedItem{}, observationFailure(FailureItem, err)
	}
	if strict {
		if err := validateObservedScope(request, name, namespace); err != nil {
			return ObservedItem{}, observationFailure(FailureScope, err)
		}
	} else if request.Namespace != "" && namespace != request.Namespace {
		return ObservedItem{}, observationFailure(FailureScope, fmt.Errorf("listed item %q has namespace %q, expected %q", name, namespace, request.Namespace))
	}

	labels, labelsPresent, labelState, err := observedLabelState(metadata)
	if err != nil {
		return ObservedItem{}, observationFailure(FailureMalformedContent, err)
	}
	if labelsPresent {
		metadata["labels"] = labels
	}
	return ObservedItem{
		Object:        unstructured.Unstructured{Object: item},
		LabelsPresent: labelsPresent,
		LabelState:    labelState,
	}, nil
}

func validateObservedGVK(item map[string]any, request ListRequest) error {
	apiVersion, apiVersionOK := item["apiVersion"].(string)
	kind, kindOK := item["kind"].(string)
	if !apiVersionOK || apiVersion == "" || !kindOK || kind == "" || apiVersion != request.APIVersion || kind != request.Kind {
		return fmt.Errorf("item GVK does not match requested %s, Kind=%s", request.APIVersion, request.Kind)
	}
	return nil
}

func observedMetadata(item map[string]any) (map[string]any, string, string, error) {
	metadata, ok := item["metadata"].(map[string]any)
	if !ok {
		return nil, "", "", fmt.Errorf("item metadata must be an object")
	}
	name, ok := metadata["name"].(string)
	if !ok || name == "" {
		return nil, "", "", fmt.Errorf("item metadata.name must be a non-empty string")
	}
	namespace, ok := metadata["namespace"].(string)
	if ok && namespace == "" {
		return nil, "", "", fmt.Errorf("item metadata.namespace must be non-empty when present")
	}
	return metadata, name, namespace, nil
}

func validateObservedScope(request ListRequest, name, namespace string) error {
	clusterScoped, supported := observedScopeForRequest(request)
	if !supported {
		return fmt.Errorf("unsupported observed GVK %s, Kind=%s", request.APIVersion, request.Kind)
	}
	if clusterScoped && namespace != "" {
		return fmt.Errorf("cluster-scoped item %q has namespace %q", name, namespace)
	}
	if !clusterScoped && (namespace == "" || (request.Namespace != "" && request.Namespace != namespace)) {
		return fmt.Errorf("namespaced item %q has invalid namespace %q", name, namespace)
	}
	return nil
}

func observedLabelState(metadata map[string]any) (map[string]any, bool, scope.LabelState, error) {
	raw, present := metadata["labels"]
	if !present {
		return nil, false, scope.LabelStateAbsent, nil
	}
	if raw == nil {
		return nil, true, scope.LabelStateInvalid, fmt.Errorf("metadata.labels must be an object")
	}
	labels, ok := raw.(map[string]any)
	if !ok {
		if stringLabels, stringOK := raw.(map[string]string); stringOK {
			labels = make(map[string]any, len(stringLabels))
			for key, value := range stringLabels {
				labels[key] = value
			}
		} else {
			return nil, true, scope.LabelStateInvalid, fmt.Errorf("metadata.labels must be an object")
		}
	}
	value, managed := labels[scope.ManagedLabelKey]
	if !managed {
		return labels, true, scope.LabelStateAbsent, nil
	}
	valueString, ok := value.(string)
	if !ok || valueString == "" {
		return labels, true, scope.LabelStateInvalid, nil
	}
	switch valueString {
	case scope.ManagedLabelValueTrue:
		return labels, true, scope.LabelStateTrue, nil
	case scope.ManagedLabelValueFalse:
		return labels, true, scope.LabelStateFalse, nil
	default:
		return labels, true, scope.LabelStateInvalid, nil
	}
}

func observedIdentity(item unstructured.Unstructured) string {
	return strings.Join([]string{
		item.GetAPIVersion(),
		item.GetKind(),
		item.GetNamespace(),
		item.GetName(),
	}, "\x00")
}

func observedScopeForRequest(request ListRequest) (bool, bool) {
	gv, err := schema.ParseGroupVersion(request.APIVersion)
	if err != nil {
		return false, false
	}
	return observedScope(gv, request.Kind)
}

func observedScope(gv schema.GroupVersion, kind string) (bool, bool) {
	if gv.Version == "" {
		return false, false
	}
	if !isSupportedObservedGVK(gv, kind) {
		return false, false
	}
	return isClusterScopedGVK(gv, kind), true
}

func isSupportedObservedGVK(gv schema.GroupVersion, kind string) bool {
	switch {
	case gv.Group == "apps" && gv.Version == "v1":
		return kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet"
	case gv.Group == "" && gv.Version == "v1":
		return kind == "Pod" || kind == "Service" || kind == "Node" || kind == "Namespace"
	default:
		return false
	}
}

func isClusterScopedGVK(gv schema.GroupVersion, kind string) bool {
	return gv.Group == "" && gv.Version == "v1" && (kind == "Node" || kind == "Namespace")
}

func failedObservation(failure ObservationFailure, err error) (ListObservation, error) {
	return ListObservation{Failure: failure}, &ObservationError{Failure: failure, Err: err}
}

func observationFailure(failure ObservationFailure, err error) error {
	return &ObservationError{Failure: failure, Err: err}
}

func errWithPage(operation string, page int, err error) error {
	return fmt.Errorf("%s page %d: %w", operation, page, err)
}
