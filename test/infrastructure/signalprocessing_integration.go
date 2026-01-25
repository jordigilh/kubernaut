package infrastructure

const (
	// SignalProcessingIntegrationPostgresPort is the host port for PostgreSQL in integration tests
	// Port allocation per DD-TEST-001 v2.2: 15436 (before Gateway 15437, HAPI 15439, Notification 15440, WE 15441)
	SignalProcessingIntegrationPostgresPort = 15436

	// SignalProcessingIntegrationRedisPort is the host port for Redis in integration tests
	// Port allocation per DD-TEST-001 v2.2: 16382 (sequential Redis allocation)
	SignalProcessingIntegrationRedisPort = 16382

	// SignalProcessingIntegrationDataStoragePort is the host port for DataStorage HTTP API in integration tests
	// Port allocation per DD-TEST-001 v2.2: 18094 (official SP allocation)
	SignalProcessingIntegrationDataStoragePort = 18094

	// SignalProcessingIntegrationMetricsPort is the host port for DataStorage metrics in integration tests
	// Port allocation per DD-TEST-001 v2.2: 19094 (follows DS pattern)
	SignalProcessingIntegrationMetricsPort = 19094
)
