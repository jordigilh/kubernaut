# Chaos Testing Tier - Gateway

**Purpose**: Test Gateway resilience under infrastructure failures and chaos scenarios  
**Coverage Target**: <5% of total tests  
**Infrastructure**: Chaos engineering tools (toxiproxy, pumba, etc.)

---

## 🔥 **Tests to Implement** (8 tests)

### **Redis Chaos Tests** (5 tests)

1. **Redis Connection Failure** (`redis_failure_test.go`)
   - Stop Redis mid-test
   - Verify Gateway returns 503
   - Verify Gateway logs error
   - Verify Gateway recovers when Redis returns

2. **Redis Recovery After Outage** (`redis_recovery_test.go`)
   - Stop Redis (Gateway returns 503)
   - Restart Redis
   - Verify Gateway automatically recovers
   - Verify Gateway resumes normal operation

3. **Redis Cluster Failover** (`redis_ha_test.go`)
   - Trigger Redis master failover
   - Verify temporary 503 errors during failover
   - Verify automatic reconnection to new master
   - Verify no data loss

4. **Redis Cluster Failover (duplicate)** (`redis_ha_test.go`)
   - Same as #3 (consolidate into one test)

5. **Redis Pipeline Failures** (`redis_pipeline_failure_test.go`)
   - Inject network failure mid-pipeline
   - Verify state consistency
   - Verify no duplicate fingerprints
   - Verify correct counts

### **K8s API Chaos Tests** (3 tests)

6. **K8s API Unavailable** (`k8s_api_failure_test.go`)
   - Simulate K8s API failure
   - Verify Gateway returns 500
   - Verify Prometheus retries
   - Verify Gateway logs error

7. **K8s API Recovery** (`k8s_api_recovery_test.go`)
   - Simulate K8s API failure
   - Restore K8s API
   - Verify Gateway automatically recovers
   - Verify pending webhooks processed

8. **K8s API Slow Responses** (`k8s_api_latency_test.go`)
   - Inject latency (5-10 seconds)
   - Verify Gateway waits (no timeout)
   - Verify CRD created successfully
   - Verify Gateway returns 201 eventually

---

## 🛠️ **Infrastructure Requirements**

### **Redis Chaos**:
- Podman/Docker control (stop/start containers)
- Redis Sentinel HA setup (master + replica + sentinel)
- Network failure injection (toxiproxy, tc, iptables)

### **K8s API Chaos**:
- K8s API failure simulation (network partition, API server stop)
- Latency injection (toxiproxy)
- ErrorInjectableK8sClient with failure modes

---

## 📊 **Estimated Effort**

- **Redis Chaos Infrastructure**: 20-30 hours
- **K8s API Chaos Infrastructure**: 10-15 hours
- **Total**: 30-45 hours

---

## 🎯 **Success Criteria**

- All 8 chaos tests passing
- Chaos infrastructure reusable for other components
- Tests run in CI/CD pipeline (nightly or on-demand)
- Clear documentation for chaos scenarios

---

**Status**: 📋 **PENDING IMPLEMENTATION**  
**Priority**: **MEDIUM** - Important for production resilience  
**Next Step**: Build chaos testing infrastructure

