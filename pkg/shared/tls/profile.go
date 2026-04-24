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

package tls

import (
	"crypto/tls"
	"fmt"
	"sync"
)

// ProfileType identifies an OpenShift TLS security profile.
// Issue #748: OCP-only — vanilla Kubernetes / Kind deployments leave this unset.
type ProfileType string

const (
	ProfileOld          ProfileType = "Old"
	ProfileIntermediate ProfileType = "Intermediate"
	ProfileModern       ProfileType = "Modern"
	ProfileCustom       ProfileType = "Custom"
)

// SecurityProfile defines TLS constraints that mirror OpenShift's
// TLSSecurityProfile without importing the OpenShift API.
// The kubernaut-operator reads the cluster APIServer CR and writes the
// resolved profile type into each service's YAML ConfigMap.
type SecurityProfile struct {
	Type             ProfileType
	MinTLSVersion    uint16
	MaxTLSVersion    uint16
	CipherSuites     []uint16
	CurvePreferences []tls.CurveID
}

// ApplyProfile overlays the given security profile constraints onto a tls.Config.
// Fields that are zero-valued in the profile are left unchanged in cfg.
// A nil profile is a no-op.
func ApplyProfile(cfg *tls.Config, profile *SecurityProfile) {
	if profile == nil || cfg == nil {
		return
	}
	if profile.MinTLSVersion != 0 {
		cfg.MinVersion = profile.MinTLSVersion
	}
	if profile.MaxTLSVersion != 0 {
		cfg.MaxVersion = profile.MaxTLSVersion
	}
	if len(profile.CipherSuites) > 0 {
		cfg.CipherSuites = profile.CipherSuites
	}
	if len(profile.CurvePreferences) > 0 {
		cfg.CurvePreferences = profile.CurvePreferences
	}
}

// intermediateCiphers are the TLS 1.2 AEAD cipher suites that align with
// Mozilla/OpenShift "Intermediate" profile. DHE suites are omitted because
// Go's crypto/tls does not support finite-field Diffie-Hellman.
var intermediateCiphers = []uint16{
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
}

var defaultCurves = []tls.CurveID{
	tls.X25519,
	tls.CurveP256,
	tls.CurveP384,
}

// OldProfile returns the "Old" TLS security profile (TLS 1.0+, broad cipher set).
// This matches OpenShift's Old profile for backward-compatible environments.
func OldProfile() *SecurityProfile {
	ciphers := make([]uint16, len(intermediateCiphers))
	copy(ciphers, intermediateCiphers)
	ciphers = append(ciphers,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	)
	curves := make([]tls.CurveID, len(defaultCurves))
	copy(curves, defaultCurves)
	return &SecurityProfile{
		Type:             ProfileOld,
		MinTLSVersion:    tls.VersionTLS10,
		CipherSuites:     ciphers,
		CurvePreferences: curves,
	}
}

// IntermediateProfile returns the "Intermediate" TLS security profile
// (TLS 1.2+, AEAD ciphers only). This is the OpenShift default.
func IntermediateProfile() *SecurityProfile {
	ciphers := make([]uint16, len(intermediateCiphers))
	copy(ciphers, intermediateCiphers)
	curves := make([]tls.CurveID, len(defaultCurves))
	copy(curves, defaultCurves)
	return &SecurityProfile{
		Type:             ProfileIntermediate,
		MinTLSVersion:    tls.VersionTLS12,
		CipherSuites:     ciphers,
		CurvePreferences: curves,
	}
}

// ModernProfile returns the "Modern" TLS security profile (TLS 1.3 only).
// Go auto-selects TLS 1.3 cipher suites, so CipherSuites is left empty.
func ModernProfile() *SecurityProfile {
	curves := make([]tls.CurveID, len(defaultCurves))
	copy(curves, defaultCurves)
	return &SecurityProfile{
		Type:             ProfileModern,
		MinTLSVersion:    tls.VersionTLS13,
		CurvePreferences: curves,
	}
}

// ProfileForType returns the built-in SecurityProfile for the given type.
// Returns nil for ProfileCustom or unrecognized types.
func ProfileForType(pt ProfileType) *SecurityProfile {
	switch pt {
	case ProfileOld:
		return OldProfile()
	case ProfileIntermediate:
		return IntermediateProfile()
	case ProfileModern:
		return ModernProfile()
	default:
		return nil
	}
}

// --- Package-level default profile (set once at startup) ---

var (
	defaultProfile *SecurityProfile
	profileMu      sync.RWMutex
)

// SetDefaultSecurityProfile stores the process-wide TLS security profile.
// Must be called before any TLS setup (ConfigureConditionalTLS, DefaultBaseTransport).
func SetDefaultSecurityProfile(p *SecurityProfile) {
	profileMu.Lock()
	defer profileMu.Unlock()
	defaultProfile = p
}

// getDefaultSecurityProfile returns the stored profile, or nil.
func getDefaultSecurityProfile() *SecurityProfile {
	profileMu.RLock()
	defer profileMu.RUnlock()
	return defaultProfile
}

// ResetDefaultSecurityProfileForTesting clears the stored profile.
// Must only be called from test code.
func ResetDefaultSecurityProfileForTesting() {
	profileMu.Lock()
	defer profileMu.Unlock()
	defaultProfile = nil
}

// SetDefaultSecurityProfileFromConfig resolves a profile type name from the
// service YAML configuration and stores it as the process-wide default.
// Empty string is a no-op (vanilla K8s / Kind where the field is omitted).
// Returns an error if profileType is non-empty but unrecognized.
func SetDefaultSecurityProfileFromConfig(profileType string) error {
	if profileType == "" {
		return nil
	}
	p := ProfileForType(ProfileType(profileType))
	if p == nil {
		return fmt.Errorf("unrecognized TLS security profile %q, falling back to default TLS 1.2", profileType)
	}
	SetDefaultSecurityProfile(p)
	return nil
}
