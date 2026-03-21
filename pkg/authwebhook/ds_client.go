/*
Copyright 2025 Jordi Gil.

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

package authwebhook

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// DSClientAdapter wraps the ogen-generated Data Storage client to implement
// WorkflowCatalogClient. This adapter translates between the admission handler's
// domain interface and the OpenAPI-generated HTTP client.
//
// ADR-058: Used by RemediationWorkflowHandler to register/disable workflows in DS.
type DSClientAdapter struct {
	client *ogenclient.Client
	logger logr.Logger
}

// NewDSClientAdapterFromClient wraps an existing ogen client as a DSClientAdapter.
// Use when the ogen client is shared across multiple adapters (e.g., audit + workflow).
func NewDSClientAdapterFromClient(client *ogenclient.Client, logger logr.Logger) *DSClientAdapter {
	return &DSClientAdapter{client: client, logger: logger}
}

// NewDSClientAdapter creates a DSClientAdapter from a Data Storage service URL.
func NewDSClientAdapter(baseURL string, timeout time.Duration, logger logr.Logger) (*DSClientAdapter, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL cannot be empty")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	baseTransport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     30 * time.Second,
	}
	transport := auth.NewServiceAccountTransportWithBase(baseTransport)

	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

	client, err := ogenclient.NewClient(baseURL, ogenclient.WithClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create ogen client: %w", err)
	}

	return &DSClientAdapter{
		client: client,
		logger: logger,
	}, nil
}

func (a *DSClientAdapter) CreateWorkflowInline(ctx context.Context, content, source, registeredBy string) (*WorkflowRegistrationResult, error) {
	req := &ogenclient.CreateWorkflowInlineRequest{
		Content: content,
	}
	req.Source.SetTo(source)
	req.RegisteredBy.SetTo(registeredBy)

	res, err := a.client.CreateWorkflow(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("data storage CreateWorkflow failed: %w", err)
	}

	switch v := res.(type) {
	case *ogenclient.CreateWorkflowCreated:
		rw := (*ogenclient.RemediationWorkflow)(v)
		return mapOgenWorkflowToResult(rw, false), nil
	case *ogenclient.CreateWorkflowOK:
		rw := (*ogenclient.RemediationWorkflow)(v)
		return mapOgenWorkflowToResult(rw, true), nil
	case *ogenclient.CreateWorkflowBadRequest:
		return nil, rfc7807Error("workflow registration rejected", (*ogenclient.RFC7807Problem)(v))
	case *ogenclient.CreateWorkflowConflict:
		return nil, rfc7807Error("workflow already exists", (*ogenclient.RFC7807Problem)(v))
	case *ogenclient.CreateWorkflowForbidden:
		return nil, rfc7807Error("workflow registration forbidden", (*ogenclient.RFC7807Problem)(v))
	case *ogenclient.CreateWorkflowUnauthorized:
		return nil, rfc7807Error("workflow registration unauthorized", (*ogenclient.RFC7807Problem)(v))
	case *ogenclient.CreateWorkflowInternalServerError:
		return nil, rfc7807Error("workflow registration server error", (*ogenclient.RFC7807Problem)(v))
	default:
		return nil, fmt.Errorf("unexpected response type from CreateWorkflow: %T", res)
	}
}

func (a *DSClientAdapter) DisableWorkflow(ctx context.Context, workflowID, reason, updatedBy string) error {
	uid, err := uuid.Parse(workflowID)
	if err != nil {
		return fmt.Errorf("invalid workflow ID %q: %w", workflowID, err)
	}

	req := &ogenclient.WorkflowLifecycleRequest{
		Reason: reason,
	}
	req.UpdatedBy.SetTo(updatedBy)

	res, disableErr := a.client.DisableWorkflow(ctx, req, ogenclient.DisableWorkflowParams{
		WorkflowID: uid,
	})
	if disableErr != nil {
		return fmt.Errorf("data storage DisableWorkflow failed: %w", disableErr)
	}

	switch v := res.(type) {
	case *ogenclient.RemediationWorkflow:
		return nil
	case *ogenclient.DisableWorkflowBadRequest:
		return rfc7807Error(fmt.Sprintf("disable workflow %q: bad request", workflowID), (*ogenclient.RFC7807Problem)(v))
	case *ogenclient.DisableWorkflowNotFound:
		return rfc7807Error(fmt.Sprintf("disable workflow %q: not found", workflowID), (*ogenclient.RFC7807Problem)(v))
	default:
		return fmt.Errorf("unexpected response type from DisableWorkflow: %T", res)
	}
}

// rfc7807Error formats an RFC 7807 Problem Details response into an actionable error message.
// DD-004: All DS error responses use this format; this helper ensures consistent extraction.
func rfc7807Error(prefix string, p *ogenclient.RFC7807Problem) error {
	return fmt.Errorf("%s: %s — %s", prefix, p.Title, p.Detail.Value)
}

func mapOgenWorkflowToResult(rw *ogenclient.RemediationWorkflow, previouslyExisted bool) *WorkflowRegistrationResult {
	return &WorkflowRegistrationResult{
		WorkflowID:        rw.WorkflowId.Value.String(),
		WorkflowName:      rw.WorkflowName,
		Version:           rw.Version,
		Status:            string(rw.Status),
		PreviouslyExisted: previouslyExisted,
	}
}

// ActionTypeRegistrationResult holds the DS response after registering or re-enabling an action type.
type ActionTypeRegistrationResult struct {
	ActionType   string
	Status       string // "created", "exists", "reenabled"
	WasReenabled bool
}

// ActionTypeDisableResult holds the DS response when disabling an action type.
type ActionTypeDisableResult struct {
	Disabled               bool
	DependentWorkflowCount int
	DependentWorkflows     []string
}

// ActionTypeUpdateResult holds the DS response after updating an action type description.
type ActionTypeUpdateResult struct {
	ActionType    string
	UpdatedFields []string
}

func (a *DSClientAdapter) CreateActionType(ctx context.Context, name string, description ogenclient.ActionTypeDescription, registeredBy string) (*ActionTypeRegistrationResult, error) {
	req := &ogenclient.ActionTypeCreateRequest{
		Name:         name,
		Description:  description,
		RegisteredBy: registeredBy,
	}

	res, err := a.client.CreateActionType(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("data storage CreateActionType failed: %w", err)
	}

	switch v := res.(type) {
	case *ogenclient.CreateActionTypeCreated:
		resp := (*ogenclient.ActionTypeCreateResponse)(v)
		return &ActionTypeRegistrationResult{
			ActionType:   resp.ActionType,
			Status:       string(resp.Status),
			WasReenabled: resp.WasReenabled,
		}, nil
	case *ogenclient.CreateActionTypeOK:
		resp := (*ogenclient.ActionTypeCreateResponse)(v)
		return &ActionTypeRegistrationResult{
			ActionType:   resp.ActionType,
			Status:       string(resp.Status),
			WasReenabled: resp.WasReenabled,
		}, nil
	default:
		return nil, fmt.Errorf("unexpected response type from CreateActionType: %T", res)
	}
}

func (a *DSClientAdapter) UpdateActionType(ctx context.Context, name string, description ogenclient.ActionTypeDescription, updatedBy string) (*ActionTypeUpdateResult, error) {
	req := &ogenclient.ActionTypeUpdateRequest{
		Description: description,
	}
	if updatedBy != "" {
		req.UpdatedBy.SetTo(updatedBy)
	}

	res, err := a.client.UpdateActionType(ctx, req, ogenclient.UpdateActionTypeParams{Name: name})
	if err != nil {
		return nil, fmt.Errorf("data storage UpdateActionType failed: %w", err)
	}

	switch v := res.(type) {
	case *ogenclient.ActionTypeUpdateResponse:
		return &ActionTypeUpdateResult{
			ActionType:    v.ActionType,
			UpdatedFields: v.UpdatedFields,
		}, nil
	default:
		return nil, fmt.Errorf("unexpected response type from UpdateActionType: %T", res)
	}
}

func (a *DSClientAdapter) DisableActionType(ctx context.Context, name string, disabledBy string) (*ActionTypeDisableResult, error) {
	req := &ogenclient.ActionTypeDisableRequest{
		DisabledBy: disabledBy,
	}

	res, err := a.client.DisableActionType(ctx, req, ogenclient.DisableActionTypeParams{Name: name})
	if err != nil {
		return nil, fmt.Errorf("data storage DisableActionType failed: %w", err)
	}

	switch v := res.(type) {
	case *ogenclient.ActionTypeDisableResponse:
		return &ActionTypeDisableResult{Disabled: true}, nil
	case *ogenclient.ActionTypeDisableDeniedResponse:
		return &ActionTypeDisableResult{
			Disabled:               false,
			DependentWorkflowCount: v.DependentWorkflowCount,
			DependentWorkflows:     v.DependentWorkflows,
		}, nil
	case *ogenclient.DisableActionTypeBadRequest:
		return nil, rfc7807Error(fmt.Sprintf("disable action type %q: bad request", name), (*ogenclient.RFC7807Problem)(v))
	case *ogenclient.DisableActionTypeNotFound:
		// Issue #469: Treat "not found" as successful cleanup — the action type
		// either never existed in DS or was already removed (e.g., empty DB after
		// helm reinstall). This unblocks CRD deletion via the validating webhook.
		return &ActionTypeDisableResult{Disabled: true}, nil
	case *ogenclient.DisableActionTypeInternalServerError:
		return nil, rfc7807Error(fmt.Sprintf("disable action type %q: server error", name), (*ogenclient.RFC7807Problem)(v))
	default:
		return nil, fmt.Errorf("unexpected response type from DisableActionType: %T", res)
	}
}

// GetActiveWorkflowCount returns the number of active workflows referencing the given action type.
// Used by the RW handler for best-effort cross-update of ActionType CRD status.activeWorkflowCount.
func (a *DSClientAdapter) GetActiveWorkflowCount(ctx context.Context, actionType string) (int, error) {
	res, err := a.client.GetActionTypeWorkflowCount(ctx, ogenclient.GetActionTypeWorkflowCountParams{Name: actionType})
	if err != nil {
		return 0, fmt.Errorf("data storage GetActionTypeWorkflowCount failed: %w", err)
	}
	return res.Count, nil
}
