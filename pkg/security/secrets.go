package security

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// SecretManager handles encrypted storage and retrieval of sensitive configuration
type SecretManager interface {
	// Secret operations
	StoreSecret(ctx context.Context, key string, value []byte) error
	RetrieveSecret(ctx context.Context, key string) ([]byte, error)
	DeleteSecret(ctx context.Context, key string) error
	ListSecrets(ctx context.Context) ([]string, error)

	// Secret rotation
	RotateSecret(ctx context.Context, key string, newValue []byte) error

	// Health and status
	IsHealthy(ctx context.Context) bool
}

// Secret represents an encrypted secret with metadata
type Secret struct {
	Key            string            `json:"key"`
	Value          []byte            `json:"-"` // Never serialize the actual value
	EncryptedValue string            `json:"encrypted_value"`
	Metadata       map[string]string `json:"metadata"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	AccessCount    int64             `json:"access_count"`
	LastAccess     time.Time         `json:"last_access"`
}

// SecretsConfig holds configuration for secrets management
type SecretsConfig struct {
	EncryptionKey    string        `yaml:"encryption_key" env:"KUBERNAUT_ENCRYPTION_KEY"`
	StorageBackend   string        `yaml:"storage_backend" default:"memory"` // memory, file, k8s
	StoragePath      string        `yaml:"storage_path" default:"./secrets"`
	RotationInterval time.Duration `yaml:"rotation_interval" default:"720h"` // 30 days
	MaxSecrets       int           `yaml:"max_secrets" default:"1000"`
	AuditAccess      bool          `yaml:"audit_access" default:"true"`
}

// MemorySecretManager provides in-memory secret storage (for development/testing)
type MemorySecretManager struct {
	secrets map[string]*Secret
	cipher  cipher.AEAD
	config  *SecretsConfig
	logger  *logrus.Logger
	mu      sync.RWMutex
}

// NewMemorySecretManager creates a new memory-based secret manager
func NewMemorySecretManager(config *SecretsConfig, logger *logrus.Logger) (*MemorySecretManager, error) {
	if config == nil {
		return nil, fmt.Errorf("secrets config is required")
	}

	// Initialize encryption
	encryptionKey := getEncryptionKey(config.EncryptionKey)
	hash := sha256.Sum256([]byte(encryptionKey))

	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	manager := &MemorySecretManager{
		secrets: make(map[string]*Secret),
		cipher:  gcm,
		config:  config,
		logger:  logger,
	}

	// Initialize with common secrets from environment
	err = manager.initializeFromEnvironment(context.Background())
	if err != nil {
		logger.WithError(err).Warn("Failed to initialize secrets from environment")
	}

	logger.Info("Initialized memory secret manager")
	return manager, nil
}

// initializeFromEnvironment loads common secrets from environment variables
func (m *MemorySecretManager) initializeFromEnvironment(ctx context.Context) error {
	commonSecrets := map[string]string{
		"k8s.kubeconfig":            "KUBECONFIG",
		"ai.openai.api_key":         "OPENAI_API_KEY",
		"ai.huggingface.token":      "HUGGINGFACE_TOKEN",
		"monitoring.prometheus.url": "PROMETHEUS_URL",
		"monitoring.grafana.token":  "GRAFANA_TOKEN",
		"database.url":              "DATABASE_URL",
		"database.password":         "DATABASE_PASSWORD",
	}

	for secretKey, envVar := range commonSecrets {
		if value := os.Getenv(envVar); value != "" {
			err := m.StoreSecret(ctx, secretKey, []byte(value))
			if err != nil {
				return fmt.Errorf("failed to store secret %s: %w", secretKey, err)
			}
		}
	}

	return nil
}

// StoreSecret encrypts and stores a secret
func (m *MemorySecretManager) StoreSecret(ctx context.Context, key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.secrets) >= m.config.MaxSecrets {
		return fmt.Errorf("maximum number of secrets (%d) reached", m.config.MaxSecrets)
	}

	// Encrypt the value
	nonce := make([]byte, m.cipher.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := m.cipher.Seal(nonce, nonce, value, []byte(key))
	encryptedValue := base64.StdEncoding.EncodeToString(ciphertext)

	// Create or update secret
	now := time.Now()
	secret := &Secret{
		Key:            key,
		EncryptedValue: encryptedValue,
		Metadata: map[string]string{
			"type":    "user_defined",
			"version": "1",
		},
		UpdatedAt: now,
	}

	if existing, exists := m.secrets[key]; exists {
		// Update existing secret
		secret.CreatedAt = existing.CreatedAt
		secret.AccessCount = existing.AccessCount
		secret.LastAccess = existing.LastAccess
		secret.Metadata = existing.Metadata // Preserve existing metadata
	} else {
		// New secret
		secret.CreatedAt = now
		secret.AccessCount = 0
	}

	m.secrets[key] = secret

	if m.config.AuditAccess {
		m.logger.WithFields(logrus.Fields{
			"secret_key": key,
			"operation":  "store",
			"timestamp":  now,
		}).Info("Secret stored")
	}

	return nil
}

// RetrieveSecret decrypts and returns a secret value
func (m *MemorySecretManager) RetrieveSecret(ctx context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	secret, exists := m.secrets[key]
	if !exists {
		return nil, fmt.Errorf("secret %s not found", key)
	}

	// Decrypt the value
	ciphertext, err := base64.StdEncoding.DecodeString(secret.EncryptedValue)
	if err != nil {
		return nil, fmt.Errorf("failed to decode secret: %w", err)
	}

	nonceSize := m.cipher.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("invalid ciphertext length")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := m.cipher.Open(nil, nonce, ciphertext, []byte(key))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secret: %w", err)
	}

	// Update access tracking
	secret.AccessCount++
	secret.LastAccess = time.Now()

	if m.config.AuditAccess {
		m.logger.WithFields(logrus.Fields{
			"secret_key":   key,
			"operation":    "retrieve",
			"access_count": secret.AccessCount,
			"timestamp":    time.Now(),
		}).Debug("Secret accessed")
	}

	return plaintext, nil
}

// DeleteSecret removes a secret
func (m *MemorySecretManager) DeleteSecret(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.secrets[key]; !exists {
		return fmt.Errorf("secret %s not found", key)
	}

	delete(m.secrets, key)

	if m.config.AuditAccess {
		m.logger.WithFields(logrus.Fields{
			"secret_key": key,
			"operation":  "delete",
			"timestamp":  time.Now(),
		}).Info("Secret deleted")
	}

	return nil
}

// ListSecrets returns all secret keys (not values)
func (m *MemorySecretManager) ListSecrets(ctx context.Context) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.secrets))
	for key := range m.secrets {
		keys = append(keys, key)
	}

	return keys, nil
}

// RotateSecret replaces an existing secret with a new value
func (m *MemorySecretManager) RotateSecret(ctx context.Context, key string, newValue []byte) error {
	// First verify the secret exists
	_, err := m.RetrieveSecret(ctx, key)
	if err != nil {
		return fmt.Errorf("cannot rotate non-existent secret: %w", err)
	}

	// Store the new value (this will update the existing secret)
	err = m.StoreSecret(ctx, key, newValue)
	if err != nil {
		return fmt.Errorf("failed to rotate secret: %w", err)
	}

	if m.config.AuditAccess {
		m.logger.WithFields(logrus.Fields{
			"secret_key": key,
			"operation":  "rotate",
			"timestamp":  time.Now(),
		}).Info("Secret rotated")
	}

	return nil
}

// IsHealthy checks if the secret manager is functioning properly
func (m *MemorySecretManager) IsHealthy(ctx context.Context) bool {
	// Simple health check - try to encrypt/decrypt test data
	testKey := "__health_check__"
	testValue := []byte("health_check_value")

	err := m.StoreSecret(ctx, testKey, testValue)
	if err != nil {
		m.logger.WithError(err).Error("Secret manager health check failed (store)")
		return false
	}

	retrieved, err := m.RetrieveSecret(ctx, testKey)
	if err != nil {
		m.logger.WithError(err).Error("Secret manager health check failed (retrieve)")
		return false
	}

	if string(retrieved) != string(testValue) {
		m.logger.Error("Secret manager health check failed (value mismatch)")
		return false
	}

	// Clean up test secret
	_ = m.DeleteSecret(ctx, testKey)

	return true
}

// Helper functions

func getEncryptionKey(configKey string) string {
	// Try config first
	if configKey != "" {
		return configKey
	}

	// Try environment variable
	if envKey := os.Getenv("KUBERNAUT_ENCRYPTION_KEY"); envKey != "" {
		return envKey
	}

	// Generate a warning key (not for production!)
	key := "WARNING_DEFAULT_ENCRYPTION_KEY_CHANGE_IN_PRODUCTION"
	return key
}

// SecretReference provides a way to reference secrets in configuration
type SecretReference struct {
	SecretKey string `json:"secret_key" yaml:"secret_key"`
	Default   string `json:"default,omitempty" yaml:"default,omitempty"`
}

// ResolveSecretReference resolves a secret reference to its actual value
func ResolveSecretReference(ctx context.Context, manager SecretManager, ref SecretReference) (string, error) {
	value, err := manager.RetrieveSecret(ctx, ref.SecretKey)
	if err != nil {
		if ref.Default != "" {
			return ref.Default, nil
		}
		return "", fmt.Errorf("failed to resolve secret reference %s: %w", ref.SecretKey, err)
	}

	return string(value), nil
}

// IsSecretReference checks if a string is a secret reference
func IsSecretReference(value string) bool {
	return strings.HasPrefix(value, "secret://")
}

// ParseSecretReference parses a secret reference from a string like "secret://key.name"
func ParseSecretReference(value string) (string, error) {
	if !IsSecretReference(value) {
		return "", fmt.Errorf("not a secret reference: %s", value)
	}

	return strings.TrimPrefix(value, "secret://"), nil
}
