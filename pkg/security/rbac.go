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

package security

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// RBAC (Role-Based Access Control) system for Kubernaut workflows and AI operations

// Permission defines a specific permission that can be granted
type Permission string

// Security permissions for Kubernaut operations
const (
	// Workflow execution permissions
	PermissionExecuteWorkflow Permission = "workflow:execute"
	PermissionCreateWorkflow  Permission = "workflow:create"
	PermissionViewWorkflow    Permission = "workflow:view"
	PermissionDeleteWorkflow  Permission = "workflow:delete"

	// Kubernetes action permissions
	PermissionRestartPod           Permission = "k8s:restart_pod"
	PermissionScaleDeployment      Permission = "k8s:scale_deployment"
	PermissionDrainNode            Permission = "k8s:drain_node"
	PermissionUpdateResourceLimits Permission = "k8s:update_resource_limits"

	// AI system permissions
	PermissionTrainModels      Permission = "ai:train_models"
	PermissionAccessInsights   Permission = "ai:access_insights"
	PermissionModifyConfidence Permission = "ai:modify_confidence"

	// Administrative permissions
	PermissionAdminAccess Permission = "admin:full_access"
	PermissionViewLogs    Permission = "admin:view_logs"
	PermissionManageUsers Permission = "admin:manage_users"
)

// Role represents a set of permissions grouped together
type Role struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Permissions []Permission `json:"permissions"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// Subject represents an entity that can have permissions (user, service, etc.)
type Subject struct {
	Type        string            `json:"type"` // "user", "service", "system"
	Identifier  string            `json:"identifier"`
	DisplayName string            `json:"display_name"`
	Attributes  map[string]string `json:"attributes"`
}

// SecurityContext represents the security context for an operation
type SecurityContext struct {
	Subject   Subject           `json:"subject"`
	Namespace string            `json:"namespace"`
	Resource  string            `json:"resource"`
	Action    string            `json:"action"`
	RequestID string            `json:"request_id"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata"`
}

// RBACProvider defines the interface for RBAC operations
type RBACProvider interface {
	// Permission checking
	HasPermission(ctx context.Context, subject Subject, permission Permission, resource string) (bool, error)
	HasRole(ctx context.Context, subject Subject, roleName string) (bool, error)

	// Role management
	GetRole(ctx context.Context, roleName string) (*Role, error)
	CreateRole(ctx context.Context, role *Role) error
	UpdateRole(ctx context.Context, role *Role) error
	DeleteRole(ctx context.Context, roleName string) error

	// Subject management
	AssignRole(ctx context.Context, subject Subject, roleName string) error
	RevokeRole(ctx context.Context, subject Subject, roleName string) error
	GetSubjectRoles(ctx context.Context, subject Subject) ([]string, error)

	// Audit
	LogAccess(ctx context.Context, securityCtx *SecurityContext, permitted bool) error
}

// DefaultRBACProvider provides a basic RBAC implementation
type DefaultRBACProvider struct {
	logger   *logrus.Logger
	roles    map[string]*Role
	bindings map[string][]string // subject -> roles
	config   *RBACConfig
}

// RBACConfig holds configuration for RBAC system
type RBACConfig struct {
	EnableAuditLogging    bool          `yaml:"enable_audit_logging" default:"true"`
	DefaultDenyPolicy     bool          `yaml:"default_deny_policy" default:"true"`
	CacheTimeout          time.Duration `yaml:"cache_timeout" default:"5m"`
	RequireAuthentication bool          `yaml:"require_authentication" default:"true"`
}

// NewDefaultRBACProvider creates a new RBAC provider with default roles
func NewDefaultRBACProvider(config *RBACConfig, logger *logrus.Logger) *DefaultRBACProvider {
	if config == nil {
		config = &RBACConfig{
			EnableAuditLogging:    true,
			DefaultDenyPolicy:     true,
			CacheTimeout:          5 * time.Minute,
			RequireAuthentication: true,
		}
	}

	provider := &DefaultRBACProvider{
		logger:   logger,
		roles:    make(map[string]*Role),
		bindings: make(map[string][]string),
		config:   config,
	}

	// Initialize with default roles
	provider.initializeDefaultRoles()

	return provider
}

// initializeDefaultRoles sets up the default role hierarchy
func (p *DefaultRBACProvider) initializeDefaultRoles() {
	now := time.Now()

	// Viewer role - read-only access
	viewerRole := &Role{
		Name:        "viewer",
		Description: "Read-only access to workflows and insights",
		Permissions: []Permission{
			PermissionViewWorkflow,
			PermissionAccessInsights,
			PermissionViewLogs,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	p.roles["viewer"] = viewerRole

	// Operator role - can execute workflows
	operatorRole := &Role{
		Name:        "operator",
		Description: "Can execute workflows and basic Kubernetes operations",
		Permissions: []Permission{
			PermissionViewWorkflow,
			PermissionExecuteWorkflow,
			PermissionRestartPod,
			PermissionScaleDeployment,
			PermissionAccessInsights,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	p.roles["operator"] = operatorRole

	// Developer role - can create and modify workflows
	developerRole := &Role{
		Name:        "developer",
		Description: "Can create, modify and execute workflows with some Kubernetes permissions",
		Permissions: []Permission{
			PermissionViewWorkflow,
			PermissionCreateWorkflow,
			PermissionExecuteWorkflow,
			PermissionRestartPod,
			PermissionScaleDeployment,
			PermissionUpdateResourceLimits,
			PermissionAccessInsights,
			PermissionModifyConfidence,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	p.roles["developer"] = developerRole

	// Admin role - full access
	adminRole := &Role{
		Name:        "admin",
		Description: "Full administrative access to all systems",
		Permissions: []Permission{
			PermissionExecuteWorkflow,
			PermissionCreateWorkflow,
			PermissionViewWorkflow,
			PermissionDeleteWorkflow,
			PermissionRestartPod,
			PermissionScaleDeployment,
			PermissionDrainNode,
			PermissionUpdateResourceLimits,
			PermissionTrainModels,
			PermissionAccessInsights,
			PermissionModifyConfidence,
			PermissionAdminAccess,
			PermissionViewLogs,
			PermissionManageUsers,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	p.roles["admin"] = adminRole

	p.logger.WithField("roles_created", len(p.roles)).Info("Initialized default RBAC roles")
}

// HasPermission checks if a subject has a specific permission for a resource
func (p *DefaultRBACProvider) HasPermission(ctx context.Context, subject Subject, permission Permission, resource string) (bool, error) {
	start := time.Now()

	// Get subject roles
	roles, err := p.GetSubjectRoles(ctx, subject)
	if err != nil {
		return false, fmt.Errorf("failed to get subject roles: %w", err)
	}

	// Check if any role has the permission
	hasPermission := false
	for _, roleName := range roles {
		role, exists := p.roles[roleName]
		if !exists {
			p.logger.WithFields(logrus.Fields{
				"role":    roleName,
				"subject": subject.Identifier,
			}).Warn("Subject has unknown role")
			continue
		}

		for _, p := range role.Permissions {
			if p == permission {
				hasPermission = true
				break
			}
		}

		if hasPermission {
			break
		}
	}

	// Log access attempt for audit
	securityCtx := &SecurityContext{
		Subject:   subject,
		Resource:  resource,
		Action:    string(permission),
		RequestID: extractRequestID(ctx),
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"check_duration": time.Since(start).String(),
		},
	}

	err = p.LogAccess(ctx, securityCtx, hasPermission)
	if err != nil {
		p.logger.WithError(err).Error("Failed to log access attempt")
	}

	return hasPermission, nil
}

// HasRole checks if a subject has a specific role
func (p *DefaultRBACProvider) HasRole(ctx context.Context, subject Subject, roleName string) (bool, error) {
	roles, err := p.GetSubjectRoles(ctx, subject)
	if err != nil {
		return false, err
	}

	for _, role := range roles {
		if role == roleName {
			return true, nil
		}
	}

	return false, nil
}

// GetRole retrieves a role definition
func (p *DefaultRBACProvider) GetRole(ctx context.Context, roleName string) (*Role, error) {
	role, exists := p.roles[roleName]
	if !exists {
		return nil, fmt.Errorf("role %s not found", roleName)
	}

	return role, nil
}

// CreateRole creates a new role
func (p *DefaultRBACProvider) CreateRole(ctx context.Context, role *Role) error {
	if _, exists := p.roles[role.Name]; exists {
		return fmt.Errorf("role %s already exists", role.Name)
	}

	role.CreatedAt = time.Now()
	role.UpdatedAt = time.Now()
	p.roles[role.Name] = role

	p.logger.WithFields(logrus.Fields{
		"role":        role.Name,
		"permissions": len(role.Permissions),
	}).Info("Created new RBAC role")

	return nil
}

// UpdateRole updates an existing role
func (p *DefaultRBACProvider) UpdateRole(ctx context.Context, role *Role) error {
	if _, exists := p.roles[role.Name]; !exists {
		return fmt.Errorf("role %s not found", role.Name)
	}

	role.UpdatedAt = time.Now()
	p.roles[role.Name] = role

	p.logger.WithField("role", role.Name).Info("Updated RBAC role")
	return nil
}

// DeleteRole removes a role
func (p *DefaultRBACProvider) DeleteRole(ctx context.Context, roleName string) error {
	if _, exists := p.roles[roleName]; !exists {
		return fmt.Errorf("role %s not found", roleName)
	}

	delete(p.roles, roleName)

	// Remove role from all subject bindings
	for subject, roles := range p.bindings {
		newRoles := make([]string, 0)
		for _, role := range roles {
			if role != roleName {
				newRoles = append(newRoles, role)
			}
		}
		p.bindings[subject] = newRoles
	}

	p.logger.WithField("role", roleName).Info("Deleted RBAC role")
	return nil
}

// AssignRole assigns a role to a subject
func (p *DefaultRBACProvider) AssignRole(ctx context.Context, subject Subject, roleName string) error {
	// Verify role exists
	if _, exists := p.roles[roleName]; !exists {
		return fmt.Errorf("role %s not found", roleName)
	}

	subjectKey := subject.Type + ":" + subject.Identifier

	// Check if already assigned
	roles := p.bindings[subjectKey]
	for _, role := range roles {
		if role == roleName {
			return nil // Already assigned
		}
	}

	// Add role
	p.bindings[subjectKey] = append(roles, roleName)

	p.logger.WithFields(logrus.Fields{
		"subject": subject.Identifier,
		"role":    roleName,
	}).Info("Assigned RBAC role to subject")

	return nil
}

// RevokeRole removes a role from a subject
func (p *DefaultRBACProvider) RevokeRole(ctx context.Context, subject Subject, roleName string) error {
	subjectKey := subject.Type + ":" + subject.Identifier

	roles := p.bindings[subjectKey]
	newRoles := make([]string, 0)

	for _, role := range roles {
		if role != roleName {
			newRoles = append(newRoles, role)
		}
	}

	p.bindings[subjectKey] = newRoles

	p.logger.WithFields(logrus.Fields{
		"subject": subject.Identifier,
		"role":    roleName,
	}).Info("Revoked RBAC role from subject")

	return nil
}

// GetSubjectRoles returns all roles assigned to a subject
func (p *DefaultRBACProvider) GetSubjectRoles(ctx context.Context, subject Subject) ([]string, error) {
	subjectKey := subject.Type + ":" + subject.Identifier
	roles := p.bindings[subjectKey]

	if len(roles) == 0 {
		// Return empty slice, not nil
		return []string{}, nil
	}

	return roles, nil
}

// LogAccess logs an access attempt for audit purposes
func (p *DefaultRBACProvider) LogAccess(ctx context.Context, securityCtx *SecurityContext, permitted bool) error {
	if !p.config.EnableAuditLogging {
		return nil
	}

	logFields := logrus.Fields{
		"subject":      securityCtx.Subject.Identifier,
		"subject_type": securityCtx.Subject.Type,
		"resource":     securityCtx.Resource,
		"action":       securityCtx.Action,
		"permitted":    permitted,
		"request_id":   securityCtx.RequestID,
		"timestamp":    securityCtx.Timestamp,
	}

	if permitted {
		p.logger.WithFields(logFields).Info("Access granted")
	} else {
		p.logger.WithFields(logFields).Warn("Access denied")
	}

	return nil
}

// Helper functions

func extractRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value("request_id").(string); ok {
		return requestID
	}
	return "unknown"
}
