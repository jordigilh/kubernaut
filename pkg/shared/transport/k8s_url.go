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

package transport

import "strings"

// K8sResourceInfo holds the parsed resource details from a K8s API URL.
type K8sResourceInfo struct {
	Verb         string
	Resource     string
	Namespace    string
	ResourceName string
}

// ParseK8sURL extracts the resource, namespace, resource name, and verb
// from a Kubernetes API server URL path and HTTP method.
//
// Supports both core API paths (/api/v1/...) and named API group paths
// (/apis/{group}/{version}/...).
func ParseK8sURL(method, path string) K8sResourceInfo {
	info := K8sResourceInfo{Verb: httpMethodToVerb(method)}

	segments := splitPath(path)

	// /api/v1/... → skip ["api", "v1"]
	// /apis/{group}/{version}/... → skip ["apis", group, version]
	var rest []string
	switch {
	case len(segments) >= 2 && segments[0] == "api":
		rest = segments[2:]
	case len(segments) >= 3 && segments[0] == "apis":
		rest = segments[3:]
	default:
		return info
	}

	// rest is one of:
	//   [resource]                          → cluster-scoped list
	//   [resource, name]                    → cluster-scoped get
	//   ["namespaces", ns, resource]        → namespaced list
	//   ["namespaces", ns, resource, name]  → namespaced get
	//   ["namespaces", ns, resource, name, subresource]
	if len(rest) >= 3 && rest[0] == "namespaces" {
		info.Namespace = rest[1]
		info.Resource = rest[2]
		if len(rest) >= 4 {
			info.ResourceName = rest[3]
		}
		if len(rest) >= 5 {
			info.Resource = rest[2] + "/" + rest[4]
		}
	} else if len(rest) >= 1 {
		info.Resource = rest[0]
		if len(rest) >= 2 {
			info.ResourceName = rest[1]
		}
	}

	return info
}

func httpMethodToVerb(method string) string {
	switch strings.ToUpper(method) {
	case "GET":
		return "get"
	case "POST":
		return "create"
	case "PUT":
		return "update"
	case "PATCH":
		return "patch"
	case "DELETE":
		return "delete"
	default:
		return strings.ToLower(method)
	}
}

func splitPath(path string) []string {
	var segments []string
	for _, s := range strings.Split(path, "/") {
		if s != "" {
			segments = append(segments, s)
		}
	}
	return segments
}
