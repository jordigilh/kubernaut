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

package fleet

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReaderFactory abstracts how client.Reader instances are obtained based on
// ClusterID. When ClusterID is empty, the local K8s client is returned.
// When non-empty, a remote MCP-backed reader for that cluster is returned.
//
// Used by GW (PrometheusAdapter), SP (K8sEnricher), and FMC (Syncer) for
// multi-cluster resource access via the MCP Gateway.
//
// BR-INTEGRATION-054, BR-INTEGRATION-065
type ReaderFactory interface {
	ReaderFor(ctx context.Context, clusterID string) (client.Reader, error)
}

// ReaderFactoryFunc adapts a plain function to the ReaderFactory interface.
// Useful for FMC's closure-based wiring where a full struct is unnecessary.
type ReaderFactoryFunc func(ctx context.Context, clusterID string) (client.Reader, error)

// ReaderFor implements ReaderFactory.
func (f ReaderFactoryFunc) ReaderFor(ctx context.Context, clusterID string) (client.Reader, error) {
	return f(ctx, clusterID)
}

type localReaderFactory struct {
	localClient client.Reader
}

// NewLocalReaderFactory creates a ReaderFactory that always returns the local
// client regardless of ClusterID. Used when MCP Gateway is not configured.
func NewLocalReaderFactory(localClient client.Reader) ReaderFactory {
	return &localReaderFactory{localClient: localClient}
}

func (f *localReaderFactory) ReaderFor(_ context.Context, _ string) (client.Reader, error) {
	return f.localClient, nil
}
