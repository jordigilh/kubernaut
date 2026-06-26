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

package mcpclient

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

type mcpReaderFactory struct {
	localClient    client.Reader
	session        *mcp.ClientSession
	prefixResolver registry.ToolPrefixResolver
}

// NewMCPReaderFactory creates a fleet.ReaderFactory that returns local clients for
// empty ClusterID and MCP-backed readers for remote clusters.
// An optional ToolPrefixResolver enables gateway-specific tool prefix lookup;
// when nil, the EAIGW "{clusterID}__" convention is used.
func NewMCPReaderFactory(localClient client.Reader, session *mcp.ClientSession, resolver ...registry.ToolPrefixResolver) fleet.ReaderFactory {
	var pr registry.ToolPrefixResolver
	if len(resolver) > 0 {
		pr = resolver[0]
	}
	return &mcpReaderFactory{
		localClient:    localClient,
		session:        session,
		prefixResolver: pr,
	}
}

func (f *mcpReaderFactory) ReaderFor(_ context.Context, clusterID string) (client.Reader, error) {
	if clusterID == "" {
		return f.localClient, nil
	}
	if f.session == nil {
		return nil, fmt.Errorf("MCP session not available for remote cluster %q", clusterID)
	}
	var opts []Option
	if f.prefixResolver != nil {
		if prefix := f.prefixResolver.ToolPrefixFor(clusterID); prefix != "" {
			opts = append(opts, WithToolPrefix(prefix))
		}
	}
	return NewFromSession(f.session, clusterID, opts...), nil
}
