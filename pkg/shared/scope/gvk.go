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
	"k8s.io/apimachinery/pkg/util/validation"
)

// ErrInvalidIdentity is the sentinel for target-identity validation failures
// (missing, partial, or malformed Group/Version/APIVersion). Callers use
// errors.Is to distinguish a permanent request defect from transient scope
// infrastructure failures. Reference: SI-10.
var ErrInvalidIdentity = errors.New("scope: invalid resource identity")

// coreVersionRE matches core (slash-less) Kubernetes API versions:
// v1, v2, v1beta1, v1alpha2, ... A slash-less value that fails this pattern
// (e.g. "apps") is a group name, not a core version, and must be rejected
// rather than accepted as core.
var coreVersionRE = regexp.MustCompile(`^v[0-9]+((alpha|beta)[0-9]+)?$`)

// ParseAPIVersion converts the Kubernetes wire representation into the group
// and version fields used by ResourceIdentity. Core resources use "v1" and
// return an empty group. An empty, malformed, or version-less value is never a
// valid identity. Grouped resources follow Kubernetes CRD version syntax
// (DNS1035 labels, e.g. "example.com/stable"); the slash-less core form keeps
// the strict v<N> rule so a group name can never masquerade as a core version.
func ParseAPIVersion(apiVersion string) (string, string, error) {
	apiVersion = strings.TrimSpace(apiVersion)
	if apiVersion == "" {
		return "", "", fmt.Errorf("%w: apiVersion is required", ErrInvalidIdentity)
	}

	// #2658: "core/v1" is a reserved alias for the core API group. Today it
	// parses as the literal group "core" — a group that matches no GVK and no
	// persisted identity row — so it is canonicalized here, at the single
	// choke point every ParseAPIVersion consumer shares. The alias set is
	// closed ("core/v1" only): core/v2 and core/v1beta1 stay invalid,
	// consistent with the core-version rejection in NormalizeAPIGroup.
	if apiVersion == "core/v1" {
		return "", "v1", nil
	}

	groupVersion, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return "", "", fmt.Errorf("%w: invalid apiVersion %q: %w", ErrInvalidIdentity, apiVersion, err)
	}
	if groupVersion.Version == "" || (strings.Contains(apiVersion, "/") && groupVersion.Group == "") {
		return "", "", fmt.Errorf("%w: invalid apiVersion %q", ErrInvalidIdentity, apiVersion)
	}
	// #2658: the literal group "core" matches no GVK and no persisted
	// identity row — every "core/<version>" spelling other than the reserved
	// "core/v1" alias (canonicalized above) is rejected.
	if groupVersion.Group == "core" {
		return "", "", fmt.Errorf("%w: invalid apiVersion %q: use the core form %q instead of the group alias", ErrInvalidIdentity, apiVersion, groupVersion.Version)
	}
	// #2658 (review R2-1): "unresolved" is the reserved unknown-identity
	// sentinel. It is a syntactically valid DNS1123 subdomain and DNS1035
	// label, so without this check "unresolved/v1" and "apps/unresolved"
	// would parse successfully and BuildTargetIdentity would emit hybrid
	// identities ("unresolved" paired with a real version, or vice versa)
	// that ParseTargetIdentity rejects — a producer gate feeding its own
	// parser unparseable output. Callers that legitimately mean "unknown"
	// pass an empty apiVersion (or the wire sentinel, handled before this
	// function); the literal word is never a real group or CRD version.
	if groupVersion.Group == scopeUnknownSentinel {
		return "", "", fmt.Errorf("%w: invalid apiVersion %q: %q is the reserved unknown-identity sentinel, not an api group", ErrInvalidIdentity, apiVersion, scopeUnknownSentinel)
	}
	if strings.Contains(apiVersion, "/") && groupVersion.Version == scopeUnknownSentinel {
		return "", "", fmt.Errorf("%w: invalid apiVersion %q: %q is the reserved unknown-identity sentinel, not an api version", ErrInvalidIdentity, apiVersion, scopeUnknownSentinel)
	}
	if !validGroupVersion(groupVersion.Group, groupVersion.Version, apiVersion) {
		return "", "", fmt.Errorf("%w: invalid apiVersion %q", ErrInvalidIdentity, apiVersion)
	}
	return groupVersion.Group, groupVersion.Version, nil
}

// validGroupVersion checks group and version syntax. The empty group is valid
// (core API) for both forms; a non-empty group must be a DNS1123 subdomain.
// Grouped versions may be any DNS1035 label (Kubernetes CRD requirement —
// e.g. "stable"); slash-less input must be a strict core v<N> version.
func validGroupVersion(group, version, original string) bool {
	if group != "" {
		if errs := validation.IsDNS1123Subdomain(group); len(errs) > 0 {
			return false
		}
	}
	if strings.Contains(original, "/") {
		return len(validation.IsDNS1035Label(version)) == 0
	}
	return coreVersionRE.MatchString(version)
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
	// Defense in depth for directly-constructed identities (SI-10): the same
	// syntax rules ParseAPIVersion enforces on the wire form apply to the
	// individual components, so a malformed group or version cannot launder
	// itself into a valid GVK through the struct fields.
	if group != "" {
		if errs := validation.IsDNS1123Subdomain(group); len(errs) > 0 {
			return schema.GroupVersionKind{}, fmt.Errorf("%w: invalid group %q: %s", ErrInvalidIdentity, group, strings.Join(errs, ", "))
		}
	}
	if len(validation.IsDNS1035Label(version)) > 0 {
		return schema.GroupVersionKind{}, fmt.Errorf("%w: invalid version %q", ErrInvalidIdentity, version)
	}

	return schema.GroupVersionKind{Group: group, Version: version, Kind: kind}, nil
}
