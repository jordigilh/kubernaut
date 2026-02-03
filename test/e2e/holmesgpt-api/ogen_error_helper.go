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

package holmesgptapi

import (
	"fmt"

	hapiclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

// CheckIncidentAnalyzeError converts ogen error response types to Go errors.
//
// ogen-generated clients return HTTP error responses (400, 422, 500) as
// RESPONSE TYPES, not as `error`. This helper converts them to proper errors
// for test assertions.
//
// Usage:
//
//	resp, err := hapiClient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(ctx, req)
//	err = CheckIncidentAnalyzeError(resp, err)
//	Expect(err).To(HaveOccurred())  // Now works correctly!
//
// Root Cause: ogen pattern documented in HAPI_E2E_OGEN_CLIENT_ISSUE_FEB_03_2026.md
func CheckIncidentAnalyzeError(resp hapiclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostRes, err error) error {
	// Network/transport errors
	if err != nil {
		return err
	}

	// Check response type for HTTP errors
	switch v := resp.(type) {
	case *hapiclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostBadRequest:
		return fmt.Errorf("HTTP 400 Bad Request: %s (status=%d)", v.GetDetail().Value, v.GetStatus())
	case *hapiclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostUnprocessableEntity:
		return fmt.Errorf("HTTP 422 Unprocessable Entity: %s (status=%d)", v.GetDetail().Value, v.GetStatus())
	case *hapiclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostUnauthorized:
		return fmt.Errorf("HTTP 401 Unauthorized: %s", v.GetDetail().Value)
	case *hapiclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostForbidden:
		return fmt.Errorf("HTTP 403 Forbidden: %s", v.GetDetail().Value)
	case *hapiclient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePostInternalServerError:
		return fmt.Errorf("HTTP 500 Internal Server Error: %s", v.GetDetail().Value)
	case *hapiclient.IncidentResponse:
		// Success - no error
		return nil
	default:
		return fmt.Errorf("unexpected response type: %T", resp)
	}
}

// CheckRecoveryAnalyzeError converts ogen error response types to Go errors for recovery endpoint.
func CheckRecoveryAnalyzeError(resp hapiclient.RecoveryAnalyzeEndpointAPIV1RecoveryAnalyzePostRes, err error) error {
	// Network/transport errors
	if err != nil {
		return err
	}

	// Check response type for HTTP errors
	switch v := resp.(type) {
	case *hapiclient.RecoveryAnalyzeEndpointAPIV1RecoveryAnalyzePostBadRequest:
		return fmt.Errorf("HTTP 400 Bad Request: %s (status=%d)", v.GetDetail().Value, v.GetStatus())
	case *hapiclient.RecoveryAnalyzeEndpointAPIV1RecoveryAnalyzePostUnprocessableEntity:
		return fmt.Errorf("HTTP 422 Unprocessable Entity: %s (status=%d)", v.GetDetail().Value, v.GetStatus())
	case *hapiclient.RecoveryAnalyzeEndpointAPIV1RecoveryAnalyzePostUnauthorized:
		return fmt.Errorf("HTTP 401 Unauthorized: %s", v.GetDetail().Value)
	case *hapiclient.RecoveryAnalyzeEndpointAPIV1RecoveryAnalyzePostForbidden:
		return fmt.Errorf("HTTP 403 Forbidden: %s", v.GetDetail().Value)
	case *hapiclient.RecoveryAnalyzeEndpointAPIV1RecoveryAnalyzePostInternalServerError:
		return fmt.Errorf("HTTP 500 Internal Server Error: %s", v.GetDetail().Value)
	case *hapiclient.RecoveryResponse:
		// Success - no error
		return nil
	default:
		return fmt.Errorf("unexpected response type: %T", resp)
	}
}
