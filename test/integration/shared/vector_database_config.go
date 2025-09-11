package shared

import (
	"fmt"
)

// VectorDatabaseTestConfig holds vector database configuration for integration tests
type VectorDatabaseTestConfig struct {
	Host     string
	Port     string
	Database string
	Username string
	Password string
	SSLMode  string
}

// LoadVectorDatabaseTestConfig loads vector database configuration from environment
func LoadVectorDatabaseTestConfig() VectorDatabaseTestConfig {
	// Default to containerized vector database ports for integration testing
	defaultPort := "5432"
	config := LoadConfig()
	if config.UseContainerDB {
		defaultPort = "5434" // Integration test vector database container port
	}

	return VectorDatabaseTestConfig{
		Host:     GetEnvOrDefault("VECTOR_DB_HOST", "localhost"),
		Port:     GetEnvOrDefault("VECTOR_DB_PORT", defaultPort),
		Database: GetEnvOrDefault("VECTOR_DB_NAME", "vector_store"),
		Username: GetEnvOrDefault("VECTOR_DB_USER", "vector_user"),
		Password: GetEnvOrDefault("VECTOR_DB_PASSWORD", "vector_password_dev"),
		SSLMode:  GetEnvOrDefault("DB_SSL_MODE", "disable"),
	}
}

// GetVectorDatabaseConnectionString returns the connection string for vector database
func (c *VectorDatabaseTestConfig) GetVectorDatabaseConnectionString() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.Username, c.Password, c.Host, c.Port, c.Database, c.SSLMode)
}

// IsContainerized returns true if we're using containerized database instances
func IsContainerized() bool {
	config := LoadConfig()
	return config.UseContainerDB
}
