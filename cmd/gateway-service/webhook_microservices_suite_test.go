package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWebhookMicroservices(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Webhook Service Microservices Integration Suite")
}
