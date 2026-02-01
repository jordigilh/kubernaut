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
	"encoding/json"
	"fmt"
	"net/http"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// RemediationRequestStatusHandler handles status updates to RemediationRequest CRDs
// BR-AUTH-001: SOC2 CC8.1 Operator Attribution (Gap #8)
// BR-AUDIT-005 v2.0: Gap #8 - TimeoutConfig mutation audit capture
// ADR-034 v1.5: webhook.remediationrequest.timeout_modified event
//
// This mutating webhook intercepts RemediationRequest status updates and:
// 1. Detects TimeoutConfig changes (old vs new)
// 2. Populates status.LastModifiedBy (operator email/username)
// 3. Populates status.LastModifiedAt (timestamp)
// 4. Writes complete audit event (WHO + WHAT + WHEN + OLD + NEW)
//
// Per Gap #8: Operators can adjust TimeoutConfig mid-remediation via kubectl edit.
// This webhook ensures all mutations are audited for SOC2 compliance.
type RemediationRequestStatusHandler struct {
	authenticator *Authenticator
	decoder       admission.Decoder
	auditStore    audit.AuditStore
}

// NewRemediationRequestStatusHandler creates a new RemediationRequest status handler
func NewRemediationRequestStatusHandler(auditStore audit.AuditStore) *RemediationRequestStatusHandler {
	return &RemediationRequestStatusHandler{
		authenticator: NewAuthenticator(),
		auditStore:    auditStore,
	}
}

// Handle processes the admission request for RemediationRequest status updates
// Implements admission.Handler interface from controller-runtime
func (h *RemediationRequestStatusHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	rr := &remediationv1.RemediationRequest{}

	// Decode the new (updated) RemediationRequest object
	err := json.Unmarshal(req.Object.Raw, rr)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode RemediationRequest: %w", err))
	}

	// Decode the old (previous) RemediationRequest object for comparison
	oldRR := &remediationv1.RemediationRequest{}
	if len(req.OldObject.Raw) > 0 {
		err = json.Unmarshal(req.OldObject.Raw, oldRR)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode old RemediationRequest: %w", err))
		}
	} else {
		// No old object (creation) - allow without modification
		return admission.Allowed("creation allowed")
	}

	// Check if TimeoutConfig changed
	if !timeoutConfigChanged(oldRR.Status.TimeoutConfig, rr.Status.TimeoutConfig) {
		// No TimeoutConfig change - allow without modification
		return admission.Allowed("no timeout config change")
	}

	// Extract authenticated user from admission request
	authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
	if err != nil {
		return admission.Denied(fmt.Sprintf("authentication required: %v", err))
	}

	// Populate authentication fields
	rr.Status.LastModifiedBy = authCtx.Username
	now := metav1.Now()
	rr.Status.LastModifiedAt = &now

	// Write complete audit event (webhook.remediationrequest.timeout_modified)
	auditEvent := audit.NewAuditEventRequest()
	audit.SetEventType(auditEvent, "webhook.remediationrequest.timeout_modified")
	audit.SetEventCategory(auditEvent, "orchestration") // Gap #8: Use orchestration category (webhook is RR implementation detail)
	audit.SetEventAction(auditEvent, "timeout_modified")
	audit.SetEventOutcome(auditEvent, audit.OutcomeSuccess)
	audit.SetActor(auditEvent, "user", authCtx.Username)
	audit.SetResource(auditEvent, "RemediationRequest", rr.Name)
	audit.SetCorrelationID(auditEvent, string(rr.UID)) // Use RR UID for correlation
	audit.SetNamespace(auditEvent, rr.Namespace)

	// Set event data payload (RemediationRequestWebhookAuditPayload)
	// Per Gap #8: Capture old and new TimeoutConfig for audit trail
	payload := api.RemediationRequestWebhookAuditPayload{
		EventType:  "webhook.remediationrequest.timeout_modified",
		RrName:     rr.Name,
		Namespace:  rr.Namespace,
		ModifiedBy: authCtx.Username, // ModifiedBy is string, not OptString
		ModifiedAt: now.Time,
	}

	// Capture old TimeoutConfig
	if oldRR.Status.TimeoutConfig != nil {
		payload.OldTimeoutConfig.SetTo(convertTimeoutConfig(oldRR.Status.TimeoutConfig))
	}

	// Capture new TimeoutConfig
	if rr.Status.TimeoutConfig != nil {
		payload.NewTimeoutConfig.SetTo(convertTimeoutConfig(rr.Status.TimeoutConfig))
	}

	auditEvent.EventData = api.NewRemediationRequestWebhookAuditPayloadAuditEventRequestEventData(payload)

	// Store audit event asynchronously (buffered write)
	// Explicitly ignore errors - audit should not block webhook operations
	// The audit store has retry + DLQ mechanisms for reliability
	_ = h.auditStore.StoreAudit(ctx, auditEvent)

	// Marshal the patched object
	marshaledRR, err := json.Marshal(rr)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to marshal patched RemediationRequest: %w", err))
	}

	// Return patched response
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledRR)
}

// InjectDecoder injects the decoder into the handler
// Required by controller-runtime admission webhook framework
func (h *RemediationRequestStatusHandler) InjectDecoder(d admission.Decoder) error {
	h.decoder = d
	return nil
}

// timeoutConfigChanged compares old and new TimeoutConfig to detect changes
func timeoutConfigChanged(old, new *remediationv1.TimeoutConfig) bool {
	// Both nil - no change
	if old == nil && new == nil {
		return false
	}

	// One nil, other not - changed
	if old == nil || new == nil {
		return true
	}

	// Compare each field
	if !durationEqual(old.Global, new.Global) {
		return true
	}
	if !durationEqual(old.Processing, new.Processing) {
		return true
	}
	if !durationEqual(old.Analyzing, new.Analyzing) {
		return true
	}
	if !durationEqual(old.Executing, new.Executing) {
		return true
	}

	return false
}

// durationEqual compares two Duration pointers
func durationEqual(a, b *metav1.Duration) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Duration == b.Duration
}

// convertTimeoutConfig converts CRD TimeoutConfig to ogen client TimeoutConfig
func convertTimeoutConfig(tc *remediationv1.TimeoutConfig) api.TimeoutConfig {
	config := api.TimeoutConfig{}

	if tc.Global != nil {
		config.Global.SetTo(tc.Global.Duration.String())
	}
	if tc.Processing != nil {
		config.Processing.SetTo(tc.Processing.Duration.String())
	}
	if tc.Analyzing != nil {
		config.Analyzing.SetTo(tc.Analyzing.Duration.String())
	}
	if tc.Executing != nil {
		config.Executing.SetTo(tc.Executing.Duration.String())
	}

	return config
}
