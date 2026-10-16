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

package config

import (
	"fmt"

	pkgconfig "github.com/jordigilh/kubernaut/pkg/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ResolveEffectiveLLM returns a complete, independently-owned LLMConfig for a
// client construction path. A nil profile resolves the investigation profile
// from static configuration plus its hot-reloadable runtime envelope. A
// non-nil profile is complete and is used directly; no field is inherited from
// the investigation profile. This is the boundary that prevents sibling
// credential and transport configuration bleed (#2515, BR-AI-090).
func ResolveEffectiveLLM(static types.LLMConfig, runtime LLMRuntimeConfig, profile *types.LLMConfig, prefix string) (types.LLMConfig, error) {
	var resolved types.LLMConfig
	if profile != nil {
		profileCopy := profile.DeepCopy()
		if profileCopy == nil {
			return resolved, fmt.Errorf("%s profile is nil", prefix)
		}
		resolved = *profileCopy
	} else {
		staticCopy := static.DeepCopy()
		if staticCopy == nil {
			return resolved, fmt.Errorf("%s static config is nil", prefix)
		}
		resolved = *staticCopy
		applyRuntimeFields(&resolved, runtime)
	}

	if err := resolved.Validate(prefix); err != nil {
		return types.LLMConfig{}, fmt.Errorf("validating %s: %w", prefix, err)
	}
	if len(resolved.CustomHeaders) > 0 {
		if err := pkgconfig.ValidateHeaderSources(resolved.CustomHeaders); err != nil {
			return types.LLMConfig{}, fmt.Errorf("validating %s.customHeaders: %w", prefix, err)
		}
	}
	if err := resolved.ResolveAPIKey(); err != nil {
		return types.LLMConfig{}, fmt.Errorf("resolving %s api key: %w", prefix, err)
	}
	if err := resolved.OAuth2.ResolveOAuth2Credentials(); err != nil {
		return types.LLMConfig{}, fmt.Errorf("resolving %s oauth2 credentials: %w", prefix, err)
	}
	return resolved, nil
}

// applyRuntimeFields overlays only the investigation runtime envelope onto a
// deep-copied static profile. Sibling profiles do not use this helper: they
// are complete profile documents and must not inherit unset fields.
func applyRuntimeFields(resolved *types.LLMConfig, runtime LLMRuntimeConfig) {
	if runtime.Model != "" {
		resolved.Model = runtime.Model
	}
	if runtime.Endpoint != "" {
		resolved.Endpoint = runtime.Endpoint
	}
	if runtime.APIKeyFile != "" {
		resolved.APIKeyFile = runtime.APIKeyFile
	}
	if runtime.APIKey != "" {
		resolved.APIKey = runtime.APIKey
	}
	if runtime.Temperature != nil {
		resolved.Temperature = runtime.Temperature
	}
	if runtime.MaxRetries != 0 {
		retries := runtime.MaxRetries
		resolved.MaxRetries = &retries
	}
	if runtime.TimeoutSeconds != 0 {
		resolved.TimeoutSeconds = runtime.TimeoutSeconds
	}
	if runtime.CustomHeaders != nil {
		resolved.CustomHeaders = append([]types.LLMHeaderDef(nil), runtime.CustomHeaders...)
	}
}
