# Makefile Cleanup Plan

## Services That Actually Exist (Keep)
- `cmd/contextapi` - Context API service
- `cmd/datastorage` - Data Storage service  
- `cmd/dynamictoolset` - Dynamic Toolset service
- `cmd/gateway` - Gateway service
- `cmd/notification` - Notification service

## Legacy Targets to Remove

### Build Targets (Lines 632-694)
- ❌ `build-all-services` - References non-existent services
- ❌ `build-alert-service` (line 646-649) - cmd/alert-service doesn't exist
- ❌ `build-workflow-service` (line 651-654) - cmd/workflow-service doesn't exist
- ❌ `build-executor-service` (line 656-659) - cmd/executor-service doesn't exist
- ❌ `build-storage-service` (line 661-664) - cmd/storage-service doesn't exist (we have datastorage)
- ❌ `build-intelligence-service` (line 666-669) - cmd/intelligence-service doesn't exist
- ❌ `build-monitor-service` (line 671-674) - cmd/monitor-service doesn't exist
- ❌ `build-context-service` (line 676-679) - cmd/context-service doesn't exist (we have contextapi)
- ❌ `build-notification-service` (line 681-684) - cmd/notification-service doesn't exist (we have notification)
- ❌ `build-ai-analysis` (line 691-694) - cmd/ai-analysis doesn't exist

### Test Targets
- ❌ `test-integration-remediation` (line 441-443) - test/integration/remediation/ deleted
- ❌ `test-integration` (line 445-447) - Too broad, replaced by service-specific targets

### Legacy Build Targets
- ❌ `build` (line 486-488) - References cmd/main.go which was deleted
- ❌ `run` (line 490-492) - References cmd/main.go which was deleted

### Docker Build Targets
- ❌ `docker-build-microservices` (line 786-787) - References non-existent services
- ❌ `docker-build-ai-analysis` (line 812-816) - cmd/ai-analysis doesn't exist
- ❌ `docker-push-microservices` (line 818-819) - References non-existent services
- ❌ `docker-push-ai-service` (line 831-835) - cmd/ai-analysis doesn't exist

## Targets to Keep (Current Architecture)

### Service-Specific Integration Tests (Keep - All Valid)
- ✅ `test-integration-datastorage` - Uses Podman (PostgreSQL)
- ✅ `test-integration-contextapi` - Uses Podman (Redis + PostgreSQL)
- ✅ `test-integration-ai` - Uses Podman (Redis) - **Note: No cmd/ai but has pkg/ai**
- ✅ `test-integration-toolset` - Uses Kind
- ✅ `test-integration-gateway-service` - Uses Kind
- ✅ `test-integration-notification` - Uses Kind
- ✅ `test-integration-service-all` - Runs all above

### Gateway Service (Keep - Valid)
- ✅ `build-gateway-service` - cmd/gateway exists
- ✅ `docker-build-gateway-service` - Valid
- ✅ `docker-push-gateway-service` - Valid
- ✅ All gateway-related targets

### Context API Service (Keep - Valid)
- ✅ All `*-context-api` targets - cmd/contextapi exists

### HolmesGPT API Service (Keep - Valid)
- ✅ All `*-holmesgpt-api` targets - holmesgpt-api/ exists

### Notification Service (Keep - Valid)
- ✅ All `test-notification-*` targets - cmd/notification exists

## Recommended Actions

1. **Remove all legacy build targets** for non-existent services
2. **Remove legacy test targets** for deleted test directories
3. **Keep all service-specific integration test targets** (they're valid)
4. **Add new build targets** for actual services:
   - `build-datastorage`
   - `build-contextapi`
   - `build-dynamictoolset`
   - `build-notification`
   - `build-gateway` (already exists)

5. **Update `build-all-services`** to only build actual services
6. **Remove `build` and `run` targets** that reference deleted cmd/main.go

