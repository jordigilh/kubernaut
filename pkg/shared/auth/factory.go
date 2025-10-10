<<<<<<< HEAD
=======
/*
Copyright 2025 Jordi Gil.

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

>>>>>>> crd_implementation
package auth

import (
	"context"
	"net/http"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/middleware"
)

// AuthConfig holds OAuth2/JWT authentication configuration for Kubernetes/OpenShift
type AuthConfig struct {
	// OAuth2/JWT is the only supported authentication method for Kubernetes/OpenShift
	OAuth2 OAuth2Config `yaml:"oauth2"`
	// Enabled determines if authentication is required (default: true for production)
	Enabled bool `yaml:"enabled"`
}

// CreateAuthenticator creates an OAuth2 authenticator for Kubernetes/OpenShift
func CreateAuthenticator(config AuthConfig, logger *logrus.Logger) (middleware.Authenticator, error) {
	// If authentication is disabled, return nil (no authentication)
	if !config.Enabled {
		logger.Warn("Authentication is disabled - this should only be used for development/testing")
		return nil, nil
	}

	// Create OAuth2/JWT authenticator using Kubernetes TokenReview API
	return NewOAuth2Authenticator(config.OAuth2, logger)
}

// NoOpAuthenticator is a no-op authenticator that always allows access
type NoOpAuthenticator struct{}

// NewNoOpAuthenticator creates a new no-op authenticator
func NewNoOpAuthenticator() *NoOpAuthenticator {
	return &NoOpAuthenticator{}
}

// Authenticate implements the Authenticator interface (always succeeds)
func (n *NoOpAuthenticator) Authenticate(ctx context.Context, r *http.Request) (*middleware.AuthenticationResult, error) {
	return &middleware.AuthenticationResult{
		Authenticated: true,
		Username:      "anonymous",
		Groups:        []string{"unauthenticated"},
		Namespace:     "",
	}, nil
}

// GetType implements the Authenticator interface
func (n *NoOpAuthenticator) GetType() string {
	return "none"
}
