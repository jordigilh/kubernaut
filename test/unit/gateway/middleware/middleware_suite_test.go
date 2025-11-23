package middleware

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestMiddleware is the single entry point for all middleware unit tests
// BR-GATEWAY-009: Rate limiting middleware
// BR-GATEWAY-018: Request logging middleware
// BR-GATEWAY-019: Error recovery middleware
// BR-GATEWAY-020: Timeout handling middleware
func TestMiddleware(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Middleware Test Suite")
}

