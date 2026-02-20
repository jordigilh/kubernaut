package config

// ControllerConfig defines controller runtime settings shared by all CRD controllers.
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase
type ControllerConfig struct {
	MetricsAddr      string `yaml:"metricsAddr"`
	HealthProbeAddr  string `yaml:"healthProbeAddr"`
	LeaderElection   bool   `yaml:"leaderElection"`
	LeaderElectionID string `yaml:"leaderElectionId"`
}
