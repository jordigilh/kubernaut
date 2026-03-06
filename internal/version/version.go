package version

// Build-time variables injected via -ldflags.
// Example: go build -ldflags "-X github.com/jordigilh/kubernaut/internal/version.Version=1.0.0"
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)
