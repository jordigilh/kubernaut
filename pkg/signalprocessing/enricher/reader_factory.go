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

package enricher

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
)

// ReaderFactory abstracts how client.Reader instances are obtained based on
// ClusterID. When ClusterID is empty, the local K8s client is returned.
// When non-empty, a remote MCP-backed reader for that cluster is returned.
// BR-INTEGRATION-054: Enables K8sEnricher to read from local or remote clusters.
type ReaderFactory interface {
	ReaderFor(ctx context.Context, clusterID string) (client.Reader, error)
}

type mcpReaderFactory struct {
	localClient client.Reader
	session     *mcp.ClientSession
}

// NewMCPReaderFactory creates a ReaderFactory that returns local clients for
// empty ClusterID and MCP-backed readers for remote clusters.
func NewMCPReaderFactory(localClient client.Reader, session *mcp.ClientSession) ReaderFactory {
	return &mcpReaderFactory{
		localClient: localClient,
		session:     session,
	}
}

func (f *mcpReaderFactory) ReaderFor(_ context.Context, clusterID string) (client.Reader, error) {
	if clusterID == "" {
		return f.localClient, nil
	}
	if f.session == nil {
		return nil, fmt.Errorf("MCP session not available for remote cluster %q", clusterID)
	}
	return mcpclient.NewFromSession(f.session, clusterID), nil
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
