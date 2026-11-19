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
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// NormalizeState is the three-state outcome of NormalizeAPIGroup (#2658):
// a resolved group (including the empty/core group), the reserved
// scope-unknown sentinel, or an invalid input. Consumers branch on the
// state, never on a sentinel group string — "unresolved" is a syntactically
// valid DNS subdomain and must not be usable as a literal group value.
type NormalizeState int

const (
	// NormalizeStateInvalid marks an input that is neither a known group
	// spelling nor the scope-unknown sentinel (SI-10: reject, do not guess).
	NormalizeStateInvalid NormalizeState = iota
	// NormalizeStateResolvedGroup marks a resolved group; the empty string
	// is the core API group.
	NormalizeStateResolvedGroup
	// NormalizeStateScopeUnknown marks the reserved "unresolved" sentinel:
	// the target's API group could not be established.
	NormalizeStateScopeUnknown
)

// kindIdentityRE is the accepted Kind shape inside an identity (Kubernetes
// kinds are title-cased alphanumerics).
var kindIdentityRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

// scopeUnknownSentinel is the reserved literal that selects the
// NormalizeStateScopeUnknown state. It is rejected as a real group by the
// builders below so no identity string can carry it as a group value.
const scopeUnknownSentinel = "unresolved"

// UnknownAPIVersionSentinel is the reserved wire value that means "the target's
// apiVersion could not be established". It is never a real apiVersion, and the
// identity renders it in both the group and version slots.
const UnknownAPIVersionSentinel = scopeUnknownSentinel

// coreGroupAlias is the reserved word that names the core API group in the
// confirmation and query-parameter vocabulary.
const coreGroupAlias = "core"

// NormalizeAPIGroup normalizes any accepted group spelling to the canonical
// group vocabulary shared by every #2658 consumer: severity-triage
// confirmation (confirmed_api_group), the DS targetApiVersion query value,
// fingerprints, and the WFE identity strings. Reference: BR-SEVERITY-001,
// BR-ORCH-042, SI-10.
//
// Precedence (a closed reserved-alias set, evaluated before the generic
// rules — an open core/<version> rule would let core/v2 bypass the group
// gate for core targets):
//
//  1. "core", "v1", "core/v1" → the empty (core) group;
//  2. "group/version" (e.g. "apps/v1") → that group;
//  3. a bare DNS1123-subdomain group (e.g. "apps") → that group;
//  4. the literal "unresolved" → NormalizeStateScopeUnknown;
//  5. anything else — including bare core-version shapes other than the
//     reserved "v1" (v2, v1beta1, ...) — → NormalizeStateInvalid.
func NormalizeAPIGroup(s string) (string, NormalizeState) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", NormalizeStateInvalid
	}
	switch s {
	case coreGroupAlias, "v1", "core/v1":
		return "", NormalizeStateResolvedGroup
	case scopeUnknownSentinel:
		return "", NormalizeStateScopeUnknown
	}
	if !strings.Contains(s, "/") {
		// A slash-less core-version shape other than the reserved "v1" (v2,
		// v1beta1, ...) is invalid: it would otherwise become a group that
		// matches no GVK and no persisted row. Otherwise a slash-less value
		// is valid only as a bare DNS1123-subdomain group.
		if coreVersionRE.MatchString(s) || len(validation.IsDNS1123Subdomain(s)) > 0 {
			return "", NormalizeStateInvalid
		}
		return s, NormalizeStateResolvedGroup
	}
	group, _, err := ParseAPIVersion(s)
	if err != nil {
		return "", NormalizeStateInvalid
	}
	return group, NormalizeStateResolvedGroup
}

// BuildTargetIdentity builds the canonical resource identity used by every
// persisted and user-visible surface (#2658):
//
//	namespace/group/version/Kind/name   namespaced
//	group/version/Kind/name             cluster-scoped (no empty segment)
//
// The API group and version are ordinary path parts of the identity — the same
// four coordinates Kubernetes itself uses (GVK + namespace + name) — so nothing
// is annotated onto the string and the identity stays parseable by splitting on
// "/". The core group renders as "core"; a target whose apiVersion cannot be
// established renders both group and version as "unresolved", so an unknown
// identity can never collide with a genuine core resource.
//
// apiVersion accepts every spelling ParseAPIVersion accepts ("apps/v1", "v1",
// "core/v1"); an empty or unparseable value yields the unresolved form.
func BuildTargetIdentity(apiVersion, namespace, kind, name string) string {
	group, version := identityGroupVersion(apiVersion)
	if namespace != "" {
		return fmt.Sprintf("%s/%s/%s/%s/%s", namespace, group, version, kind, name)
	}
	return fmt.Sprintf("%s/%s/%s/%s", group, version, kind, name)
}

// ParseTargetIdentity splits an identity built by BuildTargetIdentity back into
// its coordinates. It rejects anything that is not the 4- or 5-part form so a
// caller can never silently read a legacy or malformed string as a target.
func ParseTargetIdentity(identity string) (apiVersion, namespace, kind, name string, err error) {
	parts := strings.Split(identity, "/")
	switch len(parts) {
	case 4:
		namespace = ""
	case 5:
		// A leading empty slot ("/apps/v1/Kind/name") is a malformed 5-part
		// identity, not the equivalent cluster-scoped 4-part form — no
		// builder emits it, so it is rejected rather than silently re-read.
		if parts[0] == "" {
			return "", "", "", "", fmt.Errorf("%w: identity %q has an empty namespace slot", ErrInvalidIdentity, identity)
		}
		namespace = parts[0]
		parts = parts[1:]
	default:
		return "", "", "", "", fmt.Errorf("%w: identity %q is not a 4- or 5-part resource identity", ErrInvalidIdentity, identity)
	}
	group, version, kind, name := parts[0], parts[1], parts[2], parts[3]
	if namespace != "" {
		if errs := validation.IsDNS1123Label(namespace); len(errs) > 0 {
			return "", "", "", "", fmt.Errorf("%w: identity %q has an invalid namespace %q", ErrInvalidIdentity, identity, namespace)
		}
	}
	// Fixed arity is only meaningful if each slot is itself well-formed: a
	// mis-split path must not read as a different target.
	if !kindIdentityRE.MatchString(kind) {
		return "", "", "", "", fmt.Errorf("%w: identity %q has an invalid kind %q", ErrInvalidIdentity, identity, kind)
	}
	if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return "", "", "", "", fmt.Errorf("%w: identity %q has an invalid name %q", ErrInvalidIdentity, identity, name)
	}
	if group != coreGroupAlias && group != scopeUnknownSentinel {
		if errs := validation.IsDNS1123Subdomain(group); len(errs) > 0 {
			return "", "", "", "", fmt.Errorf("%w: identity %q has an invalid api group %q", ErrInvalidIdentity, identity, group)
		}
	}
	apiVersion, err = parseIdentityAPIVersion(identity, group, version)
	if err != nil {
		return "", "", "", "", err
	}
	return apiVersion, namespace, kind, name, nil
}

// parseIdentityAPIVersion validates the group/version pair of an identity and
// returns the canonical apiVersion it encodes. The version slot is validated
// per group, mirroring ParseAPIVersion (the producer's gate): the core group
// keeps the strict v<N> rule, while a grouped resource accepts any DNS1035
// label, which is what Kubernetes allows for CRD versions ("example.com/stable").
// Validating with the core rule alone made BuildTargetIdentity emit identities
// its own parser rejected, so such a target could never be re-read by the WFE
// validator, the routing gate or the history handoff.
func parseIdentityAPIVersion(identity, group, version string) (string, error) {
	switch {
	case group == scopeUnknownSentinel:
		// "unresolved" is all-or-nothing: a half-known identity is a hybrid no
		// builder produces, and reading it as resolvable would hide the
		// unknown group behind a real version.
		if version != scopeUnknownSentinel {
			return "", fmt.Errorf("%w: identity %q mixes the unresolved group with the api version %q", ErrInvalidIdentity, identity, version)
		}
		return "", nil
	case group == coreGroupAlias:
		if !coreVersionRE.MatchString(version) {
			return "", fmt.Errorf("%w: identity %q has an invalid core api version %q", ErrInvalidIdentity, identity, version)
		}
		return version, nil
	case version == scopeUnknownSentinel:
		return "", fmt.Errorf("%w: identity %q mixes the api group %q with the unresolved version", ErrInvalidIdentity, identity, group)
	default:
		if errs := validation.IsDNS1035Label(version); len(errs) > 0 {
			return "", fmt.Errorf("%w: identity %q has an invalid api version %q", ErrInvalidIdentity, identity, version)
		}
		return group + "/" + version, nil
	}
}

// APIGroup returns the canonical API group encoded in an identity built by
// BuildTargetIdentity: "" for the core group, "unresolved" when it could not be
// established, and the group name otherwise.
func APIGroup(identity string) string {
	if _, _, _, _, err := ParseTargetIdentity(identity); err != nil {
		return ""
	}
	// ParseTargetIdentity returns the composed apiVersion, not the group
	// slot, so read the slot directly on the already-validated identity.
	parts := strings.Split(identity, "/")
	group := parts[0]
	if len(parts) == 5 {
		group = parts[1]
	}
	if group == coreGroupAlias {
		return ""
	}
	return group
}

// WireAPIVersion renders the DataStorage targetApiVersion query value: the
// canonical apiVersion ("apps/v1", "v1") for a known target, or the reserved
// "unresolved" sentinel when it could not be established. An empty value is
// never valid on the wire because it is indistinguishable from "unknown".
func WireAPIVersion(apiVersion string) string {
	group, version, err := ParseAPIVersion(apiVersion)
	if err != nil {
		return scopeUnknownSentinel
	}
	if group == "" {
		return version
	}
	return group + "/" + version
}

// identityGroupVersion renders the group/version pair of an identity, mapping
// the core group to "core" and an unknown apiVersion to the reserved sentinel
// in both slots.
func identityGroupVersion(apiVersion string) (group, version string) {
	if apiVersion == "" {
		return scopeUnknownSentinel, scopeUnknownSentinel
	}
	parsedGroup, parsedVersion, err := ParseAPIVersion(apiVersion)
	if err != nil {
		return scopeUnknownSentinel, scopeUnknownSentinel
	}
	return DisplayAPIGroup(parsedGroup), parsedVersion
}

// DisplayAPIGroup renders a canonical group for human- and audit-facing text:
// the core (empty) group has no readable spelling of its own, so it renders as
// "core" -- the same word the confirmation field accepts.
func DisplayAPIGroup(group string) string {
	if group == "" {
		return coreGroupAlias
	}
	return group
}

// DisplayAPIGroups renders a canonical group list for human- and audit-facing
// text, falling back to fallback when no group is known (e.g. a checker that
// failed closed without identifying a specific sibling group).
func DisplayAPIGroups(groups []string, fallback string) string {
	if len(groups) == 0 {
		return fallback
	}
	rendered := make([]string, 0, len(groups))
	for _, group := range groups {
		rendered = append(rendered, DisplayAPIGroup(group))
	}
	return strings.Join(rendered, ", ")
}
