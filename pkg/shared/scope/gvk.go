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

package scope

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ParseAPIVersion converts the Kubernetes wire representation into the group
// and version fields used by ResourceIdentity. Core resources use "v1" and
// return an empty group. An empty or malformed value is never a valid explicit
// identity.
func ParseAPIVersion(apiVersion string) (string, string, error) {
	apiVersion = strings.TrimSpace(apiVersion)
	if apiVersion == "" {
		return "", "", fmt.Errorf("scope: apiVersion is required")
	}

	groupVersion, err := schema.ParseGroupVersion(apiVersion)
	if err != nil || groupVersion.Version == "" {
		if err != nil {
			return "", "", fmt.Errorf("scope: invalid apiVersion %q: %w", apiVersion, err)
		}
		return "", "", fmt.Errorf("scope: invalid apiVersion %q", apiVersion)
	}
	return groupVersion.Group, groupVersion.Version, nil
}

// ApplyAPIVersion copies an explicit Kubernetes apiVersion into a resource
// identity. An empty value is preserved for local compatibility callers; a
// remote caller must subsequently use GVKForIdentity with requireExplicit=true.
func ApplyAPIVersion(identity ResourceIdentity, apiVersion string) (ResourceIdentity, error) {
	if strings.TrimSpace(apiVersion) == "" {
		return identity, nil
	}
	group, version, err := ParseAPIVersion(apiVersion)
	if err != nil {
		return ResourceIdentity{}, err
	}
	identity.Group = group
	identity.Version = version
	return identity, nil
}

// GVKForIdentity returns the exact GVK for an identity. Local callers may
// request the historical Kind-only inference by passing requireExplicit=false.
// Remote callers must pass true: a missing or partial GVK is unavailable and
// must not be guessed from Kind.
func GVKForIdentity(identity ResourceIdentity, requireExplicit bool) (schema.GroupVersionKind, error) {
	if identity.Kind == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("scope: resource kind is required")
	}
	if identity.Group == "" && identity.Version == "" {
		if requireExplicit {
			return schema.GroupVersionKind{}, fmt.Errorf("scope: explicit group/version is required for %q", identity.Kind)
		}
		return InferGVK(identity.Kind), nil
	}
	if identity.Version == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("scope: resource version is required for %q", identity.Kind)
	}
	return schema.GroupVersionKind{
		Group:   identity.Group,
		Version: identity.Version,
		Kind:    identity.Kind,
	}, nil
}
