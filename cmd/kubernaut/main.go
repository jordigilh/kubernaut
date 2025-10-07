// ⚠️  DEPRECATED: This binary is DEPRECATED and should NOT be used
//
// According to APPROVED_MICROSERVICES_ARCHITECTURE.md, there is NO main kubernaut application.
// The system consists of 10 independent microservices:
//
// 1. 🔗 Gateway Service (Port 8080) - quay.io/jordigilh/gateway-service
// 2. 🧠 Alert Processor Service (Port 8081) - quay.io/jordigilh/alert-service
// 3. 🤖 AI Analysis Service (Port 8082) - quay.io/jordigilh/ai-service
// 4. 🎯 Workflow Orchestrator Service (Port 8083) - quay.io/jordigilh/workflow-service
// 5. ⚡ K8s Executor Service (Port 8084) - quay.io/jordigilh/executor-service
// 6. 📊 Data Storage Service (Port 8085) - quay.io/jordigilh/storage-service
// 7. 🔍 Intelligence Service (Port 8086) - quay.io/jordigilh/intelligence-service
// 8. 📈 Effectiveness Monitor Service (Port 8087) - quay.io/jordigilh/monitor-service
// 9. 🌐 Context API Service (Port 8088) - quay.io/jordigilh/context-service
// 10. 📢 Notification Service (Port 8089) - quay.io/jordigilh/notification-service
//
// DO NOT USE THIS BINARY. Use the individual microservices instead.
//
// Migration Path:
// - For AI analysis: Use cmd/ai-service/ (Port 8082)
// - For other services: Implement the respective microservice
//
// This file will be removed in a future version.

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintf(os.Stderr, `
⚠️  DEPRECATED: kubernaut main binary is DEPRECATED

According to APPROVED_MICROSERVICES_ARCHITECTURE.md, there is NO main kubernaut application.

The system consists of 10 independent microservices:

1. 🔗 Gateway Service (Port 8080) - quay.io/jordigilh/gateway-service
2. 🧠 Alert Processor Service (Port 8081) - quay.io/jordigilh/alert-service
3. 🤖 AI Analysis Service (Port 8082) - quay.io/jordigilh/ai-service
4. 🎯 Workflow Orchestrator Service (Port 8083) - quay.io/jordigilh/workflow-service
5. ⚡ K8s Executor Service (Port 8084) - quay.io/jordigilh/executor-service
6. 📊 Data Storage Service (Port 8085) - quay.io/jordigilh/storage-service
7. 🔍 Intelligence Service (Port 8086) - quay.io/jordigilh/intelligence-service
8. 📈 Effectiveness Monitor Service (Port 8087) - quay.io/jordigilh/monitor-service
9. 🌐 Context API Service (Port 8088) - quay.io/jordigilh/context-service
10. 📢 Notification Service (Port 8089) - quay.io/jordigilh/notification-service

DO NOT USE THIS BINARY. Use the individual microservices instead.

Migration Path:
- For AI analysis: Use cmd/ai-service/ (Port 8082)
- For other services: Implement the respective microservice

This binary will be removed in a future version.
`)

	os.Exit(1)
}

