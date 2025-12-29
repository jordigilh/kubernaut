package infrastructure

// GetMinimalHAPIConfig returns minimal HAPI configuration for testing
// ADR-030: Only business-critical settings exposed
func GetMinimalHAPIConfig(dataStorageURL string, logLevel string) string {
	return `logging:
  level: "` + logLevel + `"

llm:
  provider: "mock"
  model: "mock/test-model"
  endpoint: "http://localhost:11434"

data_storage:
  url: "` + dataStorageURL + `"
`
}



