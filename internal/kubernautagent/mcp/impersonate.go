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

package mcp

import (
	"errors"

	"k8s.io/client-go/rest"
)

// NewImpersonatingConfig creates a deep copy of the base rest.Config with Kubernetes
// impersonation configured for the specified user and groups. The original config
// is never mutated. The returned config uses the base config's BearerToken (SA credential)
// to authenticate to the API server while impersonating the target user.
func NewImpersonatingConfig(baseCfg *rest.Config, username string, groups []string) (*rest.Config, error) {
	if username == "" {
		return nil, errors.New("impersonating config: username must not be empty")
	}

	cfg := rest.CopyConfig(baseCfg)
	cfg.Impersonate = rest.ImpersonationConfig{
		UserName: username,
		Groups:   groups,
	}

	return cfg, nil
}
