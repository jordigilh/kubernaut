package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/jordigilh/kubernaut/pkg/shared/middleware"
)

// OAuth2Authenticator implements OAuth2/JWT authentication using Kubernetes TokenReview API
// This is the only supported authentication method for Kubernetes/OpenShift environments
type OAuth2Authenticator struct {
	client kubernetes.Interface
	config OAuth2Config
	logger *logrus.Logger
}

// OAuth2Config holds OAuth2/JWT authentication configuration for Kubernetes/OpenShift
type OAuth2Config struct {
	// Kubernetes/OpenShift cluster configuration
	KubernetesEndpoint string `yaml:"kubernetes_endpoint"`   // Kubernetes API endpoint (optional if using in-cluster)
	UseInClusterConfig bool   `yaml:"use_in_cluster_config"` // Use in-cluster config (recommended for production)

	// JWT validation settings
	Audience string `yaml:"audience"` // Expected JWT audience (e.g., "kubernaut-gateway")

	// ServiceAccount restrictions (optional but recommended for security)
	RequiredNamespace      string `yaml:"required_namespace"`       // Required ServiceAccount namespace
	RequiredServiceAccount string `yaml:"required_service_account"` // Required ServiceAccount name

	// OpenShift specific settings (future extension)
	// OpenShiftOAuthEndpoint string `yaml:"openshift_oauth_endpoint"` // OpenShift OAuth endpoint
}

// NewOAuth2Authenticator creates a new OAuth2 authenticator
func NewOAuth2Authenticator(config OAuth2Config, logger *logrus.Logger) (*OAuth2Authenticator, error) {
	client, err := createKubernetesClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return &OAuth2Authenticator{
		client: client,
		config: config,
		logger: logger,
	}, nil
}

// Authenticate implements the Authenticator interface
func (o *OAuth2Authenticator) Authenticate(ctx context.Context, r *http.Request) (*middleware.AuthenticationResult, error) {
	// Extract JWT token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return &middleware.AuthenticationResult{
			Authenticated: false,
			Errors:        []string{"missing Authorization header"},
		}, nil
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return &middleware.AuthenticationResult{
			Authenticated: false,
			Errors:        []string{"invalid Authorization header format"},
		}, nil
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		return &middleware.AuthenticationResult{
			Authenticated: false,
			Errors:        []string{"empty JWT token"},
		}, nil
	}

	// Create TokenReview request
	tokenReview := &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{
			Token: token,
		},
	}

	// Add audience if configured
	if o.config.Audience != "" {
		tokenReview.Spec.Audiences = []string{o.config.Audience}
	}

	// Call Kubernetes TokenReview API
	result, err := o.client.AuthenticationV1().TokenReviews().Create(ctx, tokenReview, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("TokenReview API call failed: %w", err)
	}

	// Check if token is authenticated
	if !result.Status.Authenticated {
		return &middleware.AuthenticationResult{
			Authenticated: false,
			Errors:        []string{fmt.Sprintf("token not authenticated: %s", result.Status.Error)},
		}, nil
	}

	userInfo := result.Status.User

	// Extract namespace from username (format: system:serviceaccount:namespace:name)
	namespace := extractNamespaceFromUsername(userInfo.Username)

	// Validate namespace restrictions if configured
	if o.config.RequiredNamespace != "" {
		if namespace != o.config.RequiredNamespace {
			return &middleware.AuthenticationResult{
				Authenticated: false,
				Errors:        []string{fmt.Sprintf("namespace mismatch: expected %s, got %s", o.config.RequiredNamespace, namespace)},
			}, nil
		}
	}

	// Validate ServiceAccount restrictions if configured
	if o.config.RequiredServiceAccount != "" {
		expectedUsername := fmt.Sprintf("system:serviceaccount:%s:%s",
			o.config.RequiredNamespace, o.config.RequiredServiceAccount)
		if userInfo.Username != expectedUsername {
			return &middleware.AuthenticationResult{
				Authenticated: false,
				Errors:        []string{fmt.Sprintf("ServiceAccount mismatch: expected %s, got %s", expectedUsername, userInfo.Username)},
			}, nil
		}
	}

	// Log successful validation
	o.logger.WithFields(logrus.Fields{
		"username":  userInfo.Username,
		"uid":       userInfo.UID,
		"groups":    userInfo.Groups,
		"namespace": namespace,
	}).Debug("OAuth2 TokenReview validation successful")

	return &middleware.AuthenticationResult{
		Authenticated: true,
		Username:      userInfo.Username,
		Groups:        userInfo.Groups,
		Namespace:     namespace,
		Metadata: map[string]string{
			"uid":      userInfo.UID,
			"audience": o.config.Audience,
		},
	}, nil
}

// GetType implements the Authenticator interface
func (o *OAuth2Authenticator) GetType() string {
	return "oauth2"
}

// createKubernetesClient creates a Kubernetes client based on configuration
func createKubernetesClient(config OAuth2Config) (kubernetes.Interface, error) {
	var restConfig *rest.Config
	var err error

	if config.UseInClusterConfig {
		// Use in-cluster configuration
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
		}
	} else if config.KubernetesEndpoint != "" {
		// Use external Kubernetes endpoint
		restConfig = &rest.Config{
			Host: config.KubernetesEndpoint,
			// TLS configuration would be added here for production
		}
	} else {
		return nil, fmt.Errorf("no Kubernetes configuration provided")
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return clientset, nil
}

// extractNamespaceFromUsername extracts namespace from Kubernetes ServiceAccount username
// Format: system:serviceaccount:namespace:name
func extractNamespaceFromUsername(username string) string {
	parts := strings.Split(username, ":")
	if len(parts) >= 3 && parts[0] == "system" && parts[1] == "serviceaccount" {
		return parts[2]
	}
	return ""
}
