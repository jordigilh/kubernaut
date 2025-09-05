package fixtures

import "github.com/jordigilh/kubernaut/pkg/infrastructure/types"

// TestCase represents a test case for alert analysis
type TestCase struct {
	Name            string
	Alert           types.Alert
	ExpectedActions []string
	MinConfidence   float64
	Description     string
}
