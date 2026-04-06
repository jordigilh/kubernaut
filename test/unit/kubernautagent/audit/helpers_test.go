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

package audit_test

import (
	"context"
	"errors"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

var errFakeOgen = errors.New("fake ogen error")

type fakeOgenClient struct {
	calls []ogenclient.AuditEventRequest
	err   error
}

func (f *fakeOgenClient) CreateAuditEvent(_ context.Context, req *ogenclient.AuditEventRequest) (ogenclient.CreateAuditEventRes, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.calls = append(f.calls, *req)
	return &ogenclient.AuditEventResponse{}, nil
}
