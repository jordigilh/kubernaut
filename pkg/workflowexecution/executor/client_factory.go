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

package executor

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// ExecutorClient is the narrow interface that executors use for K8s CRUD.
// It is satisfied by both client.Client (local) and remoteClient (MCP-backed).
// Defining this avoids requiring the full client.Client interface for MCP.
type ExecutorClient interface {
	Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
	Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error
	Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error
}

// ClientFactory provides ExecutorClient instances based on ClusterID.
// When ClusterID is empty, the local K8s client is returned.
// When non-empty, an MCP-backed composite client is returned.
//
// Authority: BR-FLEET-054, Phase 3 WE MCP executor
type ClientFactory interface {
	ClientFor(ctx context.Context, clusterID string) (ExecutorClient, error)
}

// localClientFactory always returns the injected local client.
// Used in non-fleet deployments where remote execution is disabled.
type localClientFactory struct {
	localClient client.Client
}

// NewLocalClientFactory creates a ClientFactory that only supports local execution.
func NewLocalClientFactory(c client.Client) ClientFactory {
	return &localClientFactory{localClient: c}
}

func (f *localClientFactory) ClientFor(_ context.Context, clusterID string) (ExecutorClient, error) {
	if clusterID != "" {
		return nil, fmt.Errorf("remote execution not configured: cannot target cluster %q (fleet config required)", clusterID)
	}
	return f.localClient, nil
}

// mcpClientFactory returns local clients for empty ClusterID and MCP-backed
// composite clients for remote clusters.
type mcpClientFactory struct {
	localClient    client.Client
	session        *mcp.ClientSession
	prefixResolver registry.ToolPrefixResolver
}

// NewMCPClientFactory creates a ClientFactory that supports both local and remote execution.
// An optional ToolPrefixResolver enables gateway-specific tool prefix lookup;
// when nil, the EAIGW "{clusterID}__" convention is used.
func NewMCPClientFactory(localClient client.Client, session *mcp.ClientSession, resolver ...registry.ToolPrefixResolver) ClientFactory {
	var pr registry.ToolPrefixResolver
	if len(resolver) > 0 {
		pr = resolver[0]
	}
	return &mcpClientFactory{
		localClient:    localClient,
		session:        session,
		prefixResolver: pr,
	}
}

func (f *mcpClientFactory) ClientFor(ctx context.Context, clusterID string) (ExecutorClient, error) {
	if clusterID == "" {
		return f.localClient, nil
	}
	var opts []mcpclient.Option
	if f.prefixResolver != nil {
		if prefix := f.prefixResolver.ToolPrefixFor(clusterID); prefix != "" {
			opts = append(opts, mcpclient.WithToolPrefix(prefix))
		}
	} else {
		prefix, err := mcpclient.DiscoverToolPrefix(ctx, f.session, clusterID)
		if err != nil {
			return nil, fmt.Errorf("resolve tool prefix for cluster %q: %w", clusterID, err)
		}
		opts = append(opts, mcpclient.WithToolPrefix(prefix))
	}
	reader := mcpclient.NewFromSession(f.session, clusterID, opts...)
	writer := mcpclient.NewWriterFromSession(f.session, clusterID, opts...)
	return &remoteClient{reader: reader, writer: writer}, nil
}

// remoteClient composes an MCP Reader and Writer into an ExecutorClient.
type remoteClient struct {
	reader *mcpclient.Client
	writer *mcpclient.WriterClient
}

func (r *remoteClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return r.reader.Get(ctx, key, obj, opts...)
}

func (r *remoteClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return r.writer.Create(ctx, obj, opts...)
}

func (r *remoteClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return r.writer.Delete(ctx, obj, opts...)
}
