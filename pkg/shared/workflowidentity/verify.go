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

// Package workflowidentity verifies a selected workflow against its trusted catalog.
package workflowidentity

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"time"

	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
	"github.com/jordigilh/kubernaut/pkg/shared/contenthash"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const workflowIDFieldPath = "status.workflowId"

// Verify performs a point-in-time direct server-side lookup and recomputes the
// content-derived identity. It never returns live catalog data for execution.
func Verify(ctx context.Context, reader client.Reader, namespace, workflowID string) error {
	if reader == nil || ValidateNamespace(namespace) != nil || workflowID == "" {
		return fmt.Errorf("workflow identity lookup has invalid trusted namespace, reader, or workflow ID")
	}
	backoffConfig := backoff.Config{BasePeriod: 100 * time.Millisecond, MaxPeriod: 500 * time.Millisecond, Multiplier: 2, JitterPercent: 20}
	for attempt := 1; attempt <= 3; attempt++ {
		workflows, err := listTrustedWorkflows(ctx, reader, namespace, workflowID)
		if err == nil {
			return verifyMatches(workflows, namespace, workflowID)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !isTransient(err) || attempt == 3 {
			return fmt.Errorf("lookup trusted workflow ID %q: %w", workflowID, err)
		}
		timer := time.NewTimer(backoffConfig.Calculate(int32(attempt)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return fmt.Errorf("workflow identity lookup exhausted")
}

func listTrustedWorkflows(ctx context.Context, reader client.Reader, namespace, workflowID string) ([]remediationworkflowv1.RemediationWorkflow, error) {
	var workflows remediationworkflowv1.RemediationWorkflowList
	err := reader.List(ctx, &workflows, client.InNamespace(namespace), client.MatchingFields{workflowIDFieldPath: workflowID})
	if err == nil {
		return workflows.Items, nil
	}
	if !isUnsupportedWorkflowIDSelector(err) {
		return nil, err
	}

	// Older operator releases install the workflow CRD without selectableFields.
	// Keep the server-side selector as the fast path, but preserve compatibility
	// by listing only the trusted namespace and filtering the status field locally.
	var allWorkflows remediationworkflowv1.RemediationWorkflowList
	if err := reader.List(ctx, &allWorkflows, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	matches := make([]remediationworkflowv1.RemediationWorkflow, 0, len(allWorkflows.Items))
	for _, workflow := range allWorkflows.Items {
		if workflow.Status.WorkflowID == workflowID {
			matches = append(matches, workflow)
		}
	}
	return matches, nil
}

func isUnsupportedWorkflowIDSelector(err error) bool {
	return apierrors.IsBadRequest(err) && strings.Contains(err.Error(), "field label not supported: "+workflowIDFieldPath)
}

func verifyMatches(workflows []remediationworkflowv1.RemediationWorkflow, namespace, workflowID string) error {
	if len(workflows) != 1 {
		return fmt.Errorf("workflow ID %q has %d trusted catalog matches", workflowID, len(workflows))
	}
	workflow := &workflows[0]
	if workflow.Namespace != namespace || workflow.Status.WorkflowID != workflowID {
		return fmt.Errorf("workflow ID %q does not match trusted catalog status", workflowID)
	}
	content, err := contenthash.MarshalCleanCRDContent(workflow)
	if err != nil {
		return fmt.Errorf("marshal trusted workflow %q: %w", workflowID, err)
	}
	if contenthash.DeterministicUUID(contenthash.ComputeContentHash(string(content))) != workflowID {
		return fmt.Errorf("workflow ID %q does not match trusted catalog content", workflowID)
	}
	return nil
}

// ValidateNamespace rejects a missing or malformed trusted service namespace.
func ValidateNamespace(namespace string) error {
	if problems := validation.IsDNS1123Label(namespace); len(problems) != 0 {
		return fmt.Errorf("invalid trusted workflow catalog namespace %q: %v", namespace, problems)
	}
	return nil
}

func isTransient(err error) bool {
	if apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) || apierrors.IsTooManyRequests(err) || apierrors.IsServiceUnavailable(err) || apierrors.IsInternalError(err) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError) && (networkError.Timeout() || networkError.Temporary())
}
