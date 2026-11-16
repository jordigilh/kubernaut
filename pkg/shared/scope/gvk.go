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
	"errors"
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ErrInvalidIdentity is the sentinel for target-identity validation failures
// (missing, partial, or malformed Group/Version/APIVersion). Callers use
// errors.Is to distinguish a permanent request defect from transient scope
// infrastructure failures. Reference: SI-10.
var ErrInvalidIdentity = errors.New("scope: invalid resource identity")

// versionRE matches Kubernetes API versions: v1, v2, v1beta1, v1alpha2, ...
// A slash-less value that fails this pattern (e.g. "apps") is a group name,
// not a core version, and must be rejected rather than accepted as core.
var versionRE = regexp.MustCompile(`^v[0-9]+((alpha|beta)[0-9]+)?$`)

// ParseAPIVersion converts the Kubernetes wire representation into the group
// and version fields used by ResourceIdentity. Core resources use "v1" and
// return an empty group. An empty, malformed, or version-less value is never a
// valid identity.
func ParseAPIVersion(apiVersion string) (string, string, error) {
	apiVersion = strings.TrimSpace(apiVersion)
	if apiVersion == "" {
		return "", "", fmt.Errorf("%w: apiVersion is required", ErrInvalidIdentity)
	}

	groupVersion, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return "", "", fmt.Errorf("%w: invalid apiVersion %q: %w", ErrInvalidIdentity, apiVersion, err)
	}
	if groupVersion.Version == "" || (strings.Contains(apiVersion, "/") && groupVersion.Group == "") {
		return "", "", fmt.Errorf("%w: invalid apiVersion %q", ErrInvalidIdentity, apiVersion)
	}
	if !versionRE.MatchString(groupVersion.Version) {
		return "", "", fmt.Errorf("%w: invalid apiVersion %q: version must match v<N>[alpha|beta<N>]", ErrInvalidIdentity, apiVersion)
	}
	return groupVersion.Group, groupVersion.Version, nil
}

// ApplyAPIVersion parses an explicit Kubernetes apiVersion into a resource
// identity. An empty value is an identity error: since #2592 the scope
// contract requires an explicit apiVersion, and callers must not construct a
// Kind-only identity.
func ApplyAPIVersion(identity ResourceIdentity, apiVersion string) (ResourceIdentity, error) {
	group, version, err := ParseAPIVersion(apiVersion)
	if err != nil {
		return ResourceIdentity{}, err
	}
	identity.Group = group
	identity.Version = version
	return identity, nil
}

// GVKForIdentity returns the exact GVK without inferring it from Kind.
func GVKForIdentity(identity ResourceIdentity) (schema.GroupVersionKind, error) {
	kind := strings.TrimSpace(identity.Kind)
	if kind == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("%w: resource kind is required", ErrInvalidIdentity)
	}

	group := strings.TrimSpace(identity.Group)
	version := strings.TrimSpace(identity.Version)
	if group == "" && version == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("%w: explicit group/version is required for %q", ErrInvalidIdentity, kind)
	}
	if version == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("%w: resource version is required for %q", ErrInvalidIdentity, kind)
	}

	return schema.GroupVersionKind{Group: group, Version: version, Kind: kind}, nil
}
