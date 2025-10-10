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
