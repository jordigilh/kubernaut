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

// Package fleettest provides shared test doubles for fleet.ReaderFactory.
package fleettest

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet"
)

// StubReaderFactory implements fleet.ReaderFactory for unit and integration tests.
// Configure Readers to map clusterIDs to fake client.Reader instances,
// and Err to force all ReaderFor calls to return an error.
type StubReaderFactory struct {
	Readers map[string]client.Reader
	Err     error
}

// ReaderFor returns the configured reader for the given clusterID.
func (f *StubReaderFactory) ReaderFor(_ context.Context, clusterID string) (client.Reader, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	r, ok := f.Readers[clusterID]
	if !ok {
		return nil, fmt.Errorf("fleettest: unknown cluster %q", clusterID)
	}
	return r, nil
}

var _ fleet.ReaderFactory = (*StubReaderFactory)(nil)
