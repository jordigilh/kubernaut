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

// Tool name constants matching the real kubernetes-mcp-server `core` toolset.
// These are identical for both kubernetes-mcp-server and openshift-mcp-server.
// Validated empirically in Spike S13 (tool coverage validation).
//
// The MCP Gateway exposes backend tools with a "{clusterID}__" prefix for
// multi-cluster routing. Use ClusterTool() to construct the prefixed name.
//
// Authority: Spike S13, Issue #54
const (
	ToolGet            = "resources_get"
	ToolList           = "resources_list"
	ToolCreateOrUpdate = "resources_create_or_update"
	ToolDelete         = "resources_delete"
)

// ClusterTool returns the MCP Gateway-prefixed tool name for a given cluster.
// The MCP Gateway uses the convention "{clusterID}__{toolName}" to route tool
// calls to the correct backend K8s MCP Server.
func ClusterTool(clusterID, tool string) string {
	return clusterID + "__" + tool
}
