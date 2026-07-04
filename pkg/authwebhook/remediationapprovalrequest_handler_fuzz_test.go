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

// Internal (white-box) test package: decodeRARRequest is unexported and this
// is the only way to fuzz it directly. Every other pkg/authwebhook test uses
// the external authwebhook_test package; this file is intentionally the sole
// exception, scoped narrowly to fuzzing (see AGENTS.md's "Exception: Go
// Native Fuzz Tests").
package authwebhook

import (
	"testing"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// FuzzDecodeRARRequest exercises decodeRARRequest with adversarial JSON
// bytes to surface panics in the RemediationApprovalRequest admission-webhook
// decode path. This webhook exists specifically to defend against a
// malicious/buggy caller forging status.DecidedBy (SOC2 CC8.1, CC6.8): any
// caller with RBAC to patch the status subresource controls req.Object.Raw
// and req.OldObject.Raw, making this an adversarial-by-design input surface.
//
// decodeRARRequest never touches receiver fields, so it's callable on a nil
// receiver.
//
// NOTE: native Go fuzzing (func FuzzXxx(f *testing.F)) is the sole,
// documented exception to AGENTS.md's Ginkgo/Gomega mandate -- see
// "Exception: Go Native Fuzz Tests" in AGENTS.md. This file intentionally
// contains no business-outcome assertions.
//
// Run locally with: go test ./pkg/authwebhook/ -run=^$ -fuzz=FuzzDecodeRARRequest
func FuzzDecodeRARRequest(f *testing.F) {
	seeds := []struct {
		obj, oldObj string
	}{
		{`{"metadata":{"name":"rar-1","namespace":"default"},"status":{"decision":"Approved"}}`, ``},
		{`{"status":{"decision":"Approved"}}`, `{"status":{"decision":"Pending"}}`},
		{`{}`, `{}`},
		{`not json`, ``},
		{``, ``},
		{`null`, `null`},
	}
	for _, s := range seeds {
		f.Add([]byte(s.obj), []byte(s.oldObj))
	}

	var h *RemediationApprovalRequestAuthHandler
	logger := logr.Discard()

	f.Fuzz(func(t *testing.T, rawObject, rawOldObject []byte) {
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Object:    runtime.RawExtension{Raw: rawObject},
				OldObject: runtime.RawExtension{Raw: rawOldObject},
			},
		}
		// The only contract under test: decodeRARRequest must never panic,
		// regardless of whether it accepts or rejects the payloads.
		_, _, _, _ = h.decodeRARRequest(logger, req)
	})
}
