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

package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NotificationRequestDeleteHandler handles authentication for NotificationRequest cancellation via DELETE
// BR-AUTH-001: SOC2 CC8.1 Operator Attribution
// DD-NOT-005: Immutable Spec - Cancellation via DELETE Operation
//
// This validating webhook intercepts NotificationRequest DELETE operations and adds annotations:
// - kubernaut.ai/cancelled-by (operator email/username)
// - kubernaut.ai/cancelled-at (timestamp)
//
// The annotations allow the controller to capture attribution before the CRD is removed by finalizers.
type NotificationRequestDeleteHandler struct {
	authenticator *authwebhook.Authenticator
	decoder       *admission.Decoder
}

// NewNotificationRequestDeleteHandler creates a new NotificationRequest DELETE authentication handler
func NewNotificationRequestDeleteHandler() *NotificationRequestDeleteHandler {
	return &NotificationRequestDeleteHandler{
		authenticator: authwebhook.NewAuthenticator(),
	}
}

// Handle processes the admission request for NotificationRequest DELETE
// Implements admission.Handler interface from controller-runtime
func (h *NotificationRequestDeleteHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Only handle DELETE operations
	if req.Operation != admissionv1.Delete {
		return admission.Allowed("not a DELETE operation")
	}

	nr := &notificationv1.NotificationRequest{}

	// For DELETE operations, the object to delete is in OldObject
	err := json.Unmarshal(req.OldObject.Raw, nr)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to decode NotificationRequest: %w", err))
	}

	// Extract authenticated user from admission request
	authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
	if err != nil {
		return admission.Denied(fmt.Sprintf("authentication required: %v", err))
	}

	// Initialize annotations map if it doesn't exist
	if nr.Annotations == nil {
		nr.Annotations = make(map[string]string)
	}

	// Check if cancellation is already attributed (idempotent)
	if _, exists := nr.Annotations["kubernaut.ai/cancelled-by"]; exists {
		// Already attributed - allow without modification
		return admission.Allowed("cancellation already attributed")
	}

	// Add cancellation attribution annotations
	nr.Annotations["kubernaut.ai/cancelled-by"] = authCtx.Username
	nr.Annotations["kubernaut.ai/cancelled-at"] = time.Now().UTC().Format(time.RFC3339)

	// For validating webhooks handling DELETE, we need to patch the object
	// before it's deleted so the controller can read the annotations
	marshaledNR, err := json.Marshal(nr)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to marshal patched NotificationRequest: %w", err))
	}

	// Return patched response
	// Note: For DELETE operations, we patch the OldObject
	return admission.PatchResponseFromRaw(req.OldObject.Raw, marshaledNR)
}

// InjectDecoder injects the decoder into the handler
// Required by controller-runtime admission webhook framework
func (h *NotificationRequestDeleteHandler) InjectDecoder(d *admission.Decoder) error {
	h.decoder = d
	return nil
}

