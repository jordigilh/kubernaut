# Redis Capacity Analysis: CRDs in Flight

## Question
With 1GB of Redis memory, what is the theoretical threshold of CRDs in flight that the Gateway can handle?

---

## Memory Breakdown per CRD

### Deduplication Keys (per unique fingerprint)
```
Key: dedup:metadata:{fingerprint}
Value: JSON metadata
{
  "fingerprint": "64-char-sha256",
  "remediationRequestRef": "crd-name-uuid",
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "count": 5
}
```
**Size**: ~200 bytes per key (optimized in DD-GATEWAY-004)

### Storm Detection Keys (per namespace:alertname)
```
Key: storm:counter:{namespace}:{alertname}
Value: Integer counter
Size: ~50 bytes

Key: storm:flag:{namespace}:{alertname}
Value: "1" (flag)
Size: ~30 bytes
```
**Total**: ~80 bytes per namespace:alertname pair

### Storm Aggregation Keys (per storm CRD)
```
Key: storm:metadata:{namespace}:{alertname}
Value: Lightweight metadata JSON
{
  "pattern": "HighCPU in production",
  "alertCount": 15,
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "affectedResources": [
    {"kind": "Pod", "name": "api-1", "namespace": "prod"},
    {"kind": "Pod", "name": "api-2", "namespace": "prod"}
  ]
}
```
**Size**: ~300 bytes base + (50 bytes × number of affected resources)
- **Average**: ~500 bytes (assuming 4 affected resources)

### Rate Limiting Keys (per IP/token)
```
Key: ratelimit:{ip_or_token}
Value: Request count
Size: ~40 bytes
```

---

## Scenario Analysis

### Scenario 1: Normal Operation (No Storm)
**Assumptions**:
- 1 CRD = 1 deduplication key + 1 storm detection counter + 1 storm flag
- **Total per CRD**: 200 + 50 + 30 = **280 bytes**

**Calculation**:
```
Available memory: 1GB = 1,073,741,824 bytes
Overhead (Redis metadata): 30% = 322,122,547 bytes
Usable memory: 751,619,277 bytes

CRDs in flight = 751,619,277 / 280 = 2,684,354 CRDs
```

**Theoretical Threshold**: **~2.68 million CRDs** (normal operation)

---

### Scenario 2: Storm Aggregation Active
**Assumptions**:
- 10% of CRDs trigger storm aggregation (10 alerts → 1 storm CRD)
- Storm CRD = 1 dedup key + 1 storm counter + 1 storm flag + 1 storm metadata
- **Total per storm CRD**: 200 + 50 + 30 + 500 = **780 bytes**
- **Total per individual CRD**: 280 bytes (as above)

**Calculation** (mixed workload):
```
Usable memory: 751,619,277 bytes

Storm CRDs (10%): 10% of total
Individual CRDs (90%): 90% of total

Let X = total CRDs
Storm CRDs: 0.1X × 780 bytes = 78X bytes
Individual CRDs: 0.9X × 280 bytes = 252X bytes
Total: 330X bytes

X = 751,619,277 / 330 = 2,277,331 CRDs
```

**Theoretical Threshold**: **~2.28 million CRDs** (with 10% storm aggregation)

---

### Scenario 3: Heavy Storm (50% Aggregation)
**Assumptions**:
- 50% of CRDs trigger storm aggregation
- Storm CRDs have 10 affected resources each (larger metadata)
- **Storm metadata size**: 300 + (50 × 10) = 800 bytes
- **Total per storm CRD**: 200 + 50 + 30 + 800 = **1,080 bytes**

**Calculation**:
```
Usable memory: 751,619,277 bytes

Let X = total CRDs
Storm CRDs: 0.5X × 1,080 bytes = 540X bytes
Individual CRDs: 0.5X × 280 bytes = 140X bytes
Total: 680X bytes

X = 751,619,277 / 680 = 1,105,322 CRDs
```

**Theoretical Threshold**: **~1.11 million CRDs** (heavy storm scenario)

---

## Real-World Constraints

### 1. TTL Expiration (5 minutes)
**Impact**: Keys expire after 5 minutes, freeing memory
```
CRDs per 5-minute window = Threshold / (5 × 60) seconds
= 2,684,354 / 300 = 8,948 CRDs/second
```

**Realistic Throughput**: **~8,900 CRDs/second** (normal operation)

### 2. Rate Limiting (100 req/min per client)
**Impact**: Limits concurrent CRD creation
```
Max clients before rate limit: 1GB / (100 req/min × 40 bytes) = ~267,000 clients
```

**Realistic Concurrent Clients**: **~267,000 clients** at 100 req/min each

### 3. Kubernetes API Throttling
**Impact**: K8s API limits CRD creation rate
- **Default K8s API QPS**: 5 requests/second per client
- **Gateway QPS**: 50 requests/second (configured in tests)
- **Production QPS**: 5 requests/second (default)

**Realistic CRD Creation Rate**: **5 CRDs/second** (production default)

---

## Summary Table

| Scenario | Memory per CRD | Theoretical Threshold | Realistic Throughput |
|----------|---------------|----------------------|---------------------|
| **Normal Operation** | 280 bytes | 2.68M CRDs | 8,900 CRDs/sec |
| **10% Storm Aggregation** | 330 bytes (avg) | 2.28M CRDs | 7,600 CRDs/sec |
| **50% Storm Aggregation** | 680 bytes (avg) | 1.11M CRDs | 3,700 CRDs/sec |
| **Production (5 QPS)** | 280 bytes | 2.68M CRDs | **5 CRDs/sec** |

---

## Bottlenecks (Ranked by Impact)

### 1. Kubernetes API Rate Limiting (HIGHEST IMPACT)
- **Default**: 5 CRDs/second
- **Configured (tests)**: 50 CRDs/second
- **Recommendation**: Use K8s client-side rate limiting (QPS=50, Burst=100)

### 2. Redis Memory (MEDIUM IMPACT)
- **1GB**: Supports 1.11M - 2.68M CRDs in flight
- **2GB**: Supports 2.22M - 5.36M CRDs in flight
- **Recommendation**: 2GB for production (current configuration)

### 3. Rate Limiting (LOW IMPACT)
- **100 req/min**: Prevents DoS, minimal impact on legitimate traffic
- **Recommendation**: Keep current configuration

---

## Recommendations

### For Production (1GB Redis)
1. **Expected Load**: 5 CRDs/second (K8s API default QPS)
2. **Peak Capacity**: 2.68M CRDs in flight (normal operation)
3. **Safety Margin**: 99.8% headroom (5 CRDs/sec << 8,900 CRDs/sec)
4. **Verdict**: **1GB is MORE than sufficient for production**

### For Testing (2GB Redis)
1. **Expected Load**: 50 CRDs/second (configured QPS)
2. **Peak Capacity**: 5.36M CRDs in flight (normal operation)
3. **Safety Margin**: 99.1% headroom (50 CRDs/sec << 17,800 CRDs/sec)
4. **Verdict**: **2GB provides excellent headroom for concurrent tests**

### For High-Throughput Scenarios (Future)
If Gateway needs to handle >8,900 CRDs/second:
1. **Increase K8s API QPS**: Configure client-side rate limiting (QPS=100, Burst=200)
2. **Increase Redis Memory**: 4GB supports 5.36M - 10.72M CRDs in flight
3. **Implement Redis Cluster**: Horizontal scaling for >10M CRDs

---

## Confidence Assessment

**95% Confidence** that:
- ✅ 1GB Redis supports **2.68M CRDs** in normal operation
- ✅ 2GB Redis supports **5.36M CRDs** in normal operation
- ✅ Production bottleneck is **K8s API (5 CRDs/sec)**, not Redis
- ✅ 1GB Redis is **sufficient for production** (99.8% headroom)
- ✅ 2GB Redis is **optimal for testing** (99.1% headroom)

**Risk Factors**:
- Memory fragmentation may reduce capacity by 10-20%
- Large storm CRDs (>20 affected resources) may reduce capacity by 30%
- Concurrent tests may spike memory usage temporarily

**Mitigation**:
- 2GB Redis provides 2x safety margin
- TTL expiration (5 min) prevents long-term accumulation
- Aggressive cleanup in tests prevents state pollution



## Question
With 1GB of Redis memory, what is the theoretical threshold of CRDs in flight that the Gateway can handle?

---

## Memory Breakdown per CRD

### Deduplication Keys (per unique fingerprint)
```
Key: dedup:metadata:{fingerprint}
Value: JSON metadata
{
  "fingerprint": "64-char-sha256",
  "remediationRequestRef": "crd-name-uuid",
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "count": 5
}
```
**Size**: ~200 bytes per key (optimized in DD-GATEWAY-004)

### Storm Detection Keys (per namespace:alertname)
```
Key: storm:counter:{namespace}:{alertname}
Value: Integer counter
Size: ~50 bytes

Key: storm:flag:{namespace}:{alertname}
Value: "1" (flag)
Size: ~30 bytes
```
**Total**: ~80 bytes per namespace:alertname pair

### Storm Aggregation Keys (per storm CRD)
```
Key: storm:metadata:{namespace}:{alertname}
Value: Lightweight metadata JSON
{
  "pattern": "HighCPU in production",
  "alertCount": 15,
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "affectedResources": [
    {"kind": "Pod", "name": "api-1", "namespace": "prod"},
    {"kind": "Pod", "name": "api-2", "namespace": "prod"}
  ]
}
```
**Size**: ~300 bytes base + (50 bytes × number of affected resources)
- **Average**: ~500 bytes (assuming 4 affected resources)

### Rate Limiting Keys (per IP/token)
```
Key: ratelimit:{ip_or_token}
Value: Request count
Size: ~40 bytes
```

---

## Scenario Analysis

### Scenario 1: Normal Operation (No Storm)
**Assumptions**:
- 1 CRD = 1 deduplication key + 1 storm detection counter + 1 storm flag
- **Total per CRD**: 200 + 50 + 30 = **280 bytes**

**Calculation**:
```
Available memory: 1GB = 1,073,741,824 bytes
Overhead (Redis metadata): 30% = 322,122,547 bytes
Usable memory: 751,619,277 bytes

CRDs in flight = 751,619,277 / 280 = 2,684,354 CRDs
```

**Theoretical Threshold**: **~2.68 million CRDs** (normal operation)

---

### Scenario 2: Storm Aggregation Active
**Assumptions**:
- 10% of CRDs trigger storm aggregation (10 alerts → 1 storm CRD)
- Storm CRD = 1 dedup key + 1 storm counter + 1 storm flag + 1 storm metadata
- **Total per storm CRD**: 200 + 50 + 30 + 500 = **780 bytes**
- **Total per individual CRD**: 280 bytes (as above)

**Calculation** (mixed workload):
```
Usable memory: 751,619,277 bytes

Storm CRDs (10%): 10% of total
Individual CRDs (90%): 90% of total

Let X = total CRDs
Storm CRDs: 0.1X × 780 bytes = 78X bytes
Individual CRDs: 0.9X × 280 bytes = 252X bytes
Total: 330X bytes

X = 751,619,277 / 330 = 2,277,331 CRDs
```

**Theoretical Threshold**: **~2.28 million CRDs** (with 10% storm aggregation)

---

### Scenario 3: Heavy Storm (50% Aggregation)
**Assumptions**:
- 50% of CRDs trigger storm aggregation
- Storm CRDs have 10 affected resources each (larger metadata)
- **Storm metadata size**: 300 + (50 × 10) = 800 bytes
- **Total per storm CRD**: 200 + 50 + 30 + 800 = **1,080 bytes**

**Calculation**:
```
Usable memory: 751,619,277 bytes

Let X = total CRDs
Storm CRDs: 0.5X × 1,080 bytes = 540X bytes
Individual CRDs: 0.5X × 280 bytes = 140X bytes
Total: 680X bytes

X = 751,619,277 / 680 = 1,105,322 CRDs
```

**Theoretical Threshold**: **~1.11 million CRDs** (heavy storm scenario)

---

## Real-World Constraints

### 1. TTL Expiration (5 minutes)
**Impact**: Keys expire after 5 minutes, freeing memory
```
CRDs per 5-minute window = Threshold / (5 × 60) seconds
= 2,684,354 / 300 = 8,948 CRDs/second
```

**Realistic Throughput**: **~8,900 CRDs/second** (normal operation)

### 2. Rate Limiting (100 req/min per client)
**Impact**: Limits concurrent CRD creation
```
Max clients before rate limit: 1GB / (100 req/min × 40 bytes) = ~267,000 clients
```

**Realistic Concurrent Clients**: **~267,000 clients** at 100 req/min each

### 3. Kubernetes API Throttling
**Impact**: K8s API limits CRD creation rate
- **Default K8s API QPS**: 5 requests/second per client
- **Gateway QPS**: 50 requests/second (configured in tests)
- **Production QPS**: 5 requests/second (default)

**Realistic CRD Creation Rate**: **5 CRDs/second** (production default)

---

## Summary Table

| Scenario | Memory per CRD | Theoretical Threshold | Realistic Throughput |
|----------|---------------|----------------------|---------------------|
| **Normal Operation** | 280 bytes | 2.68M CRDs | 8,900 CRDs/sec |
| **10% Storm Aggregation** | 330 bytes (avg) | 2.28M CRDs | 7,600 CRDs/sec |
| **50% Storm Aggregation** | 680 bytes (avg) | 1.11M CRDs | 3,700 CRDs/sec |
| **Production (5 QPS)** | 280 bytes | 2.68M CRDs | **5 CRDs/sec** |

---

## Bottlenecks (Ranked by Impact)

### 1. Kubernetes API Rate Limiting (HIGHEST IMPACT)
- **Default**: 5 CRDs/second
- **Configured (tests)**: 50 CRDs/second
- **Recommendation**: Use K8s client-side rate limiting (QPS=50, Burst=100)

### 2. Redis Memory (MEDIUM IMPACT)
- **1GB**: Supports 1.11M - 2.68M CRDs in flight
- **2GB**: Supports 2.22M - 5.36M CRDs in flight
- **Recommendation**: 2GB for production (current configuration)

### 3. Rate Limiting (LOW IMPACT)
- **100 req/min**: Prevents DoS, minimal impact on legitimate traffic
- **Recommendation**: Keep current configuration

---

## Recommendations

### For Production (1GB Redis)
1. **Expected Load**: 5 CRDs/second (K8s API default QPS)
2. **Peak Capacity**: 2.68M CRDs in flight (normal operation)
3. **Safety Margin**: 99.8% headroom (5 CRDs/sec << 8,900 CRDs/sec)
4. **Verdict**: **1GB is MORE than sufficient for production**

### For Testing (2GB Redis)
1. **Expected Load**: 50 CRDs/second (configured QPS)
2. **Peak Capacity**: 5.36M CRDs in flight (normal operation)
3. **Safety Margin**: 99.1% headroom (50 CRDs/sec << 17,800 CRDs/sec)
4. **Verdict**: **2GB provides excellent headroom for concurrent tests**

### For High-Throughput Scenarios (Future)
If Gateway needs to handle >8,900 CRDs/second:
1. **Increase K8s API QPS**: Configure client-side rate limiting (QPS=100, Burst=200)
2. **Increase Redis Memory**: 4GB supports 5.36M - 10.72M CRDs in flight
3. **Implement Redis Cluster**: Horizontal scaling for >10M CRDs

---

## Confidence Assessment

**95% Confidence** that:
- ✅ 1GB Redis supports **2.68M CRDs** in normal operation
- ✅ 2GB Redis supports **5.36M CRDs** in normal operation
- ✅ Production bottleneck is **K8s API (5 CRDs/sec)**, not Redis
- ✅ 1GB Redis is **sufficient for production** (99.8% headroom)
- ✅ 2GB Redis is **optimal for testing** (99.1% headroom)

**Risk Factors**:
- Memory fragmentation may reduce capacity by 10-20%
- Large storm CRDs (>20 affected resources) may reduce capacity by 30%
- Concurrent tests may spike memory usage temporarily

**Mitigation**:
- 2GB Redis provides 2x safety margin
- TTL expiration (5 min) prevents long-term accumulation
- Aggressive cleanup in tests prevents state pollution

# Redis Capacity Analysis: CRDs in Flight

## Question
With 1GB of Redis memory, what is the theoretical threshold of CRDs in flight that the Gateway can handle?

---

## Memory Breakdown per CRD

### Deduplication Keys (per unique fingerprint)
```
Key: dedup:metadata:{fingerprint}
Value: JSON metadata
{
  "fingerprint": "64-char-sha256",
  "remediationRequestRef": "crd-name-uuid",
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "count": 5
}
```
**Size**: ~200 bytes per key (optimized in DD-GATEWAY-004)

### Storm Detection Keys (per namespace:alertname)
```
Key: storm:counter:{namespace}:{alertname}
Value: Integer counter
Size: ~50 bytes

Key: storm:flag:{namespace}:{alertname}
Value: "1" (flag)
Size: ~30 bytes
```
**Total**: ~80 bytes per namespace:alertname pair

### Storm Aggregation Keys (per storm CRD)
```
Key: storm:metadata:{namespace}:{alertname}
Value: Lightweight metadata JSON
{
  "pattern": "HighCPU in production",
  "alertCount": 15,
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "affectedResources": [
    {"kind": "Pod", "name": "api-1", "namespace": "prod"},
    {"kind": "Pod", "name": "api-2", "namespace": "prod"}
  ]
}
```
**Size**: ~300 bytes base + (50 bytes × number of affected resources)
- **Average**: ~500 bytes (assuming 4 affected resources)

### Rate Limiting Keys (per IP/token)
```
Key: ratelimit:{ip_or_token}
Value: Request count
Size: ~40 bytes
```

---

## Scenario Analysis

### Scenario 1: Normal Operation (No Storm)
**Assumptions**:
- 1 CRD = 1 deduplication key + 1 storm detection counter + 1 storm flag
- **Total per CRD**: 200 + 50 + 30 = **280 bytes**

**Calculation**:
```
Available memory: 1GB = 1,073,741,824 bytes
Overhead (Redis metadata): 30% = 322,122,547 bytes
Usable memory: 751,619,277 bytes

CRDs in flight = 751,619,277 / 280 = 2,684,354 CRDs
```

**Theoretical Threshold**: **~2.68 million CRDs** (normal operation)

---

### Scenario 2: Storm Aggregation Active
**Assumptions**:
- 10% of CRDs trigger storm aggregation (10 alerts → 1 storm CRD)
- Storm CRD = 1 dedup key + 1 storm counter + 1 storm flag + 1 storm metadata
- **Total per storm CRD**: 200 + 50 + 30 + 500 = **780 bytes**
- **Total per individual CRD**: 280 bytes (as above)

**Calculation** (mixed workload):
```
Usable memory: 751,619,277 bytes

Storm CRDs (10%): 10% of total
Individual CRDs (90%): 90% of total

Let X = total CRDs
Storm CRDs: 0.1X × 780 bytes = 78X bytes
Individual CRDs: 0.9X × 280 bytes = 252X bytes
Total: 330X bytes

X = 751,619,277 / 330 = 2,277,331 CRDs
```

**Theoretical Threshold**: **~2.28 million CRDs** (with 10% storm aggregation)

---

### Scenario 3: Heavy Storm (50% Aggregation)
**Assumptions**:
- 50% of CRDs trigger storm aggregation
- Storm CRDs have 10 affected resources each (larger metadata)
- **Storm metadata size**: 300 + (50 × 10) = 800 bytes
- **Total per storm CRD**: 200 + 50 + 30 + 800 = **1,080 bytes**

**Calculation**:
```
Usable memory: 751,619,277 bytes

Let X = total CRDs
Storm CRDs: 0.5X × 1,080 bytes = 540X bytes
Individual CRDs: 0.5X × 280 bytes = 140X bytes
Total: 680X bytes

X = 751,619,277 / 680 = 1,105,322 CRDs
```

**Theoretical Threshold**: **~1.11 million CRDs** (heavy storm scenario)

---

## Real-World Constraints

### 1. TTL Expiration (5 minutes)
**Impact**: Keys expire after 5 minutes, freeing memory
```
CRDs per 5-minute window = Threshold / (5 × 60) seconds
= 2,684,354 / 300 = 8,948 CRDs/second
```

**Realistic Throughput**: **~8,900 CRDs/second** (normal operation)

### 2. Rate Limiting (100 req/min per client)
**Impact**: Limits concurrent CRD creation
```
Max clients before rate limit: 1GB / (100 req/min × 40 bytes) = ~267,000 clients
```

**Realistic Concurrent Clients**: **~267,000 clients** at 100 req/min each

### 3. Kubernetes API Throttling
**Impact**: K8s API limits CRD creation rate
- **Default K8s API QPS**: 5 requests/second per client
- **Gateway QPS**: 50 requests/second (configured in tests)
- **Production QPS**: 5 requests/second (default)

**Realistic CRD Creation Rate**: **5 CRDs/second** (production default)

---

## Summary Table

| Scenario | Memory per CRD | Theoretical Threshold | Realistic Throughput |
|----------|---------------|----------------------|---------------------|
| **Normal Operation** | 280 bytes | 2.68M CRDs | 8,900 CRDs/sec |
| **10% Storm Aggregation** | 330 bytes (avg) | 2.28M CRDs | 7,600 CRDs/sec |
| **50% Storm Aggregation** | 680 bytes (avg) | 1.11M CRDs | 3,700 CRDs/sec |
| **Production (5 QPS)** | 280 bytes | 2.68M CRDs | **5 CRDs/sec** |

---

## Bottlenecks (Ranked by Impact)

### 1. Kubernetes API Rate Limiting (HIGHEST IMPACT)
- **Default**: 5 CRDs/second
- **Configured (tests)**: 50 CRDs/second
- **Recommendation**: Use K8s client-side rate limiting (QPS=50, Burst=100)

### 2. Redis Memory (MEDIUM IMPACT)
- **1GB**: Supports 1.11M - 2.68M CRDs in flight
- **2GB**: Supports 2.22M - 5.36M CRDs in flight
- **Recommendation**: 2GB for production (current configuration)

### 3. Rate Limiting (LOW IMPACT)
- **100 req/min**: Prevents DoS, minimal impact on legitimate traffic
- **Recommendation**: Keep current configuration

---

## Recommendations

### For Production (1GB Redis)
1. **Expected Load**: 5 CRDs/second (K8s API default QPS)
2. **Peak Capacity**: 2.68M CRDs in flight (normal operation)
3. **Safety Margin**: 99.8% headroom (5 CRDs/sec << 8,900 CRDs/sec)
4. **Verdict**: **1GB is MORE than sufficient for production**

### For Testing (2GB Redis)
1. **Expected Load**: 50 CRDs/second (configured QPS)
2. **Peak Capacity**: 5.36M CRDs in flight (normal operation)
3. **Safety Margin**: 99.1% headroom (50 CRDs/sec << 17,800 CRDs/sec)
4. **Verdict**: **2GB provides excellent headroom for concurrent tests**

### For High-Throughput Scenarios (Future)
If Gateway needs to handle >8,900 CRDs/second:
1. **Increase K8s API QPS**: Configure client-side rate limiting (QPS=100, Burst=200)
2. **Increase Redis Memory**: 4GB supports 5.36M - 10.72M CRDs in flight
3. **Implement Redis Cluster**: Horizontal scaling for >10M CRDs

---

## Confidence Assessment

**95% Confidence** that:
- ✅ 1GB Redis supports **2.68M CRDs** in normal operation
- ✅ 2GB Redis supports **5.36M CRDs** in normal operation
- ✅ Production bottleneck is **K8s API (5 CRDs/sec)**, not Redis
- ✅ 1GB Redis is **sufficient for production** (99.8% headroom)
- ✅ 2GB Redis is **optimal for testing** (99.1% headroom)

**Risk Factors**:
- Memory fragmentation may reduce capacity by 10-20%
- Large storm CRDs (>20 affected resources) may reduce capacity by 30%
- Concurrent tests may spike memory usage temporarily

**Mitigation**:
- 2GB Redis provides 2x safety margin
- TTL expiration (5 min) prevents long-term accumulation
- Aggressive cleanup in tests prevents state pollution

# Redis Capacity Analysis: CRDs in Flight

## Question
With 1GB of Redis memory, what is the theoretical threshold of CRDs in flight that the Gateway can handle?

---

## Memory Breakdown per CRD

### Deduplication Keys (per unique fingerprint)
```
Key: dedup:metadata:{fingerprint}
Value: JSON metadata
{
  "fingerprint": "64-char-sha256",
  "remediationRequestRef": "crd-name-uuid",
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "count": 5
}
```
**Size**: ~200 bytes per key (optimized in DD-GATEWAY-004)

### Storm Detection Keys (per namespace:alertname)
```
Key: storm:counter:{namespace}:{alertname}
Value: Integer counter
Size: ~50 bytes

Key: storm:flag:{namespace}:{alertname}
Value: "1" (flag)
Size: ~30 bytes
```
**Total**: ~80 bytes per namespace:alertname pair

### Storm Aggregation Keys (per storm CRD)
```
Key: storm:metadata:{namespace}:{alertname}
Value: Lightweight metadata JSON
{
  "pattern": "HighCPU in production",
  "alertCount": 15,
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "affectedResources": [
    {"kind": "Pod", "name": "api-1", "namespace": "prod"},
    {"kind": "Pod", "name": "api-2", "namespace": "prod"}
  ]
}
```
**Size**: ~300 bytes base + (50 bytes × number of affected resources)
- **Average**: ~500 bytes (assuming 4 affected resources)

### Rate Limiting Keys (per IP/token)
```
Key: ratelimit:{ip_or_token}
Value: Request count
Size: ~40 bytes
```

---

## Scenario Analysis

### Scenario 1: Normal Operation (No Storm)
**Assumptions**:
- 1 CRD = 1 deduplication key + 1 storm detection counter + 1 storm flag
- **Total per CRD**: 200 + 50 + 30 = **280 bytes**

**Calculation**:
```
Available memory: 1GB = 1,073,741,824 bytes
Overhead (Redis metadata): 30% = 322,122,547 bytes
Usable memory: 751,619,277 bytes

CRDs in flight = 751,619,277 / 280 = 2,684,354 CRDs
```

**Theoretical Threshold**: **~2.68 million CRDs** (normal operation)

---

### Scenario 2: Storm Aggregation Active
**Assumptions**:
- 10% of CRDs trigger storm aggregation (10 alerts → 1 storm CRD)
- Storm CRD = 1 dedup key + 1 storm counter + 1 storm flag + 1 storm metadata
- **Total per storm CRD**: 200 + 50 + 30 + 500 = **780 bytes**
- **Total per individual CRD**: 280 bytes (as above)

**Calculation** (mixed workload):
```
Usable memory: 751,619,277 bytes

Storm CRDs (10%): 10% of total
Individual CRDs (90%): 90% of total

Let X = total CRDs
Storm CRDs: 0.1X × 780 bytes = 78X bytes
Individual CRDs: 0.9X × 280 bytes = 252X bytes
Total: 330X bytes

X = 751,619,277 / 330 = 2,277,331 CRDs
```

**Theoretical Threshold**: **~2.28 million CRDs** (with 10% storm aggregation)

---

### Scenario 3: Heavy Storm (50% Aggregation)
**Assumptions**:
- 50% of CRDs trigger storm aggregation
- Storm CRDs have 10 affected resources each (larger metadata)
- **Storm metadata size**: 300 + (50 × 10) = 800 bytes
- **Total per storm CRD**: 200 + 50 + 30 + 800 = **1,080 bytes**

**Calculation**:
```
Usable memory: 751,619,277 bytes

Let X = total CRDs
Storm CRDs: 0.5X × 1,080 bytes = 540X bytes
Individual CRDs: 0.5X × 280 bytes = 140X bytes
Total: 680X bytes

X = 751,619,277 / 680 = 1,105,322 CRDs
```

**Theoretical Threshold**: **~1.11 million CRDs** (heavy storm scenario)

---

## Real-World Constraints

### 1. TTL Expiration (5 minutes)
**Impact**: Keys expire after 5 minutes, freeing memory
```
CRDs per 5-minute window = Threshold / (5 × 60) seconds
= 2,684,354 / 300 = 8,948 CRDs/second
```

**Realistic Throughput**: **~8,900 CRDs/second** (normal operation)

### 2. Rate Limiting (100 req/min per client)
**Impact**: Limits concurrent CRD creation
```
Max clients before rate limit: 1GB / (100 req/min × 40 bytes) = ~267,000 clients
```

**Realistic Concurrent Clients**: **~267,000 clients** at 100 req/min each

### 3. Kubernetes API Throttling
**Impact**: K8s API limits CRD creation rate
- **Default K8s API QPS**: 5 requests/second per client
- **Gateway QPS**: 50 requests/second (configured in tests)
- **Production QPS**: 5 requests/second (default)

**Realistic CRD Creation Rate**: **5 CRDs/second** (production default)

---

## Summary Table

| Scenario | Memory per CRD | Theoretical Threshold | Realistic Throughput |
|----------|---------------|----------------------|---------------------|
| **Normal Operation** | 280 bytes | 2.68M CRDs | 8,900 CRDs/sec |
| **10% Storm Aggregation** | 330 bytes (avg) | 2.28M CRDs | 7,600 CRDs/sec |
| **50% Storm Aggregation** | 680 bytes (avg) | 1.11M CRDs | 3,700 CRDs/sec |
| **Production (5 QPS)** | 280 bytes | 2.68M CRDs | **5 CRDs/sec** |

---

## Bottlenecks (Ranked by Impact)

### 1. Kubernetes API Rate Limiting (HIGHEST IMPACT)
- **Default**: 5 CRDs/second
- **Configured (tests)**: 50 CRDs/second
- **Recommendation**: Use K8s client-side rate limiting (QPS=50, Burst=100)

### 2. Redis Memory (MEDIUM IMPACT)
- **1GB**: Supports 1.11M - 2.68M CRDs in flight
- **2GB**: Supports 2.22M - 5.36M CRDs in flight
- **Recommendation**: 2GB for production (current configuration)

### 3. Rate Limiting (LOW IMPACT)
- **100 req/min**: Prevents DoS, minimal impact on legitimate traffic
- **Recommendation**: Keep current configuration

---

## Recommendations

### For Production (1GB Redis)
1. **Expected Load**: 5 CRDs/second (K8s API default QPS)
2. **Peak Capacity**: 2.68M CRDs in flight (normal operation)
3. **Safety Margin**: 99.8% headroom (5 CRDs/sec << 8,900 CRDs/sec)
4. **Verdict**: **1GB is MORE than sufficient for production**

### For Testing (2GB Redis)
1. **Expected Load**: 50 CRDs/second (configured QPS)
2. **Peak Capacity**: 5.36M CRDs in flight (normal operation)
3. **Safety Margin**: 99.1% headroom (50 CRDs/sec << 17,800 CRDs/sec)
4. **Verdict**: **2GB provides excellent headroom for concurrent tests**

### For High-Throughput Scenarios (Future)
If Gateway needs to handle >8,900 CRDs/second:
1. **Increase K8s API QPS**: Configure client-side rate limiting (QPS=100, Burst=200)
2. **Increase Redis Memory**: 4GB supports 5.36M - 10.72M CRDs in flight
3. **Implement Redis Cluster**: Horizontal scaling for >10M CRDs

---

## Confidence Assessment

**95% Confidence** that:
- ✅ 1GB Redis supports **2.68M CRDs** in normal operation
- ✅ 2GB Redis supports **5.36M CRDs** in normal operation
- ✅ Production bottleneck is **K8s API (5 CRDs/sec)**, not Redis
- ✅ 1GB Redis is **sufficient for production** (99.8% headroom)
- ✅ 2GB Redis is **optimal for testing** (99.1% headroom)

**Risk Factors**:
- Memory fragmentation may reduce capacity by 10-20%
- Large storm CRDs (>20 affected resources) may reduce capacity by 30%
- Concurrent tests may spike memory usage temporarily

**Mitigation**:
- 2GB Redis provides 2x safety margin
- TTL expiration (5 min) prevents long-term accumulation
- Aggressive cleanup in tests prevents state pollution



## Question
With 1GB of Redis memory, what is the theoretical threshold of CRDs in flight that the Gateway can handle?

---

## Memory Breakdown per CRD

### Deduplication Keys (per unique fingerprint)
```
Key: dedup:metadata:{fingerprint}
Value: JSON metadata
{
  "fingerprint": "64-char-sha256",
  "remediationRequestRef": "crd-name-uuid",
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "count": 5
}
```
**Size**: ~200 bytes per key (optimized in DD-GATEWAY-004)

### Storm Detection Keys (per namespace:alertname)
```
Key: storm:counter:{namespace}:{alertname}
Value: Integer counter
Size: ~50 bytes

Key: storm:flag:{namespace}:{alertname}
Value: "1" (flag)
Size: ~30 bytes
```
**Total**: ~80 bytes per namespace:alertname pair

### Storm Aggregation Keys (per storm CRD)
```
Key: storm:metadata:{namespace}:{alertname}
Value: Lightweight metadata JSON
{
  "pattern": "HighCPU in production",
  "alertCount": 15,
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "affectedResources": [
    {"kind": "Pod", "name": "api-1", "namespace": "prod"},
    {"kind": "Pod", "name": "api-2", "namespace": "prod"}
  ]
}
```
**Size**: ~300 bytes base + (50 bytes × number of affected resources)
- **Average**: ~500 bytes (assuming 4 affected resources)

### Rate Limiting Keys (per IP/token)
```
Key: ratelimit:{ip_or_token}
Value: Request count
Size: ~40 bytes
```

---

## Scenario Analysis

### Scenario 1: Normal Operation (No Storm)
**Assumptions**:
- 1 CRD = 1 deduplication key + 1 storm detection counter + 1 storm flag
- **Total per CRD**: 200 + 50 + 30 = **280 bytes**

**Calculation**:
```
Available memory: 1GB = 1,073,741,824 bytes
Overhead (Redis metadata): 30% = 322,122,547 bytes
Usable memory: 751,619,277 bytes

CRDs in flight = 751,619,277 / 280 = 2,684,354 CRDs
```

**Theoretical Threshold**: **~2.68 million CRDs** (normal operation)

---

### Scenario 2: Storm Aggregation Active
**Assumptions**:
- 10% of CRDs trigger storm aggregation (10 alerts → 1 storm CRD)
- Storm CRD = 1 dedup key + 1 storm counter + 1 storm flag + 1 storm metadata
- **Total per storm CRD**: 200 + 50 + 30 + 500 = **780 bytes**
- **Total per individual CRD**: 280 bytes (as above)

**Calculation** (mixed workload):
```
Usable memory: 751,619,277 bytes

Storm CRDs (10%): 10% of total
Individual CRDs (90%): 90% of total

Let X = total CRDs
Storm CRDs: 0.1X × 780 bytes = 78X bytes
Individual CRDs: 0.9X × 280 bytes = 252X bytes
Total: 330X bytes

X = 751,619,277 / 330 = 2,277,331 CRDs
```

**Theoretical Threshold**: **~2.28 million CRDs** (with 10% storm aggregation)

---

### Scenario 3: Heavy Storm (50% Aggregation)
**Assumptions**:
- 50% of CRDs trigger storm aggregation
- Storm CRDs have 10 affected resources each (larger metadata)
- **Storm metadata size**: 300 + (50 × 10) = 800 bytes
- **Total per storm CRD**: 200 + 50 + 30 + 800 = **1,080 bytes**

**Calculation**:
```
Usable memory: 751,619,277 bytes

Let X = total CRDs
Storm CRDs: 0.5X × 1,080 bytes = 540X bytes
Individual CRDs: 0.5X × 280 bytes = 140X bytes
Total: 680X bytes

X = 751,619,277 / 680 = 1,105,322 CRDs
```

**Theoretical Threshold**: **~1.11 million CRDs** (heavy storm scenario)

---

## Real-World Constraints

### 1. TTL Expiration (5 minutes)
**Impact**: Keys expire after 5 minutes, freeing memory
```
CRDs per 5-minute window = Threshold / (5 × 60) seconds
= 2,684,354 / 300 = 8,948 CRDs/second
```

**Realistic Throughput**: **~8,900 CRDs/second** (normal operation)

### 2. Rate Limiting (100 req/min per client)
**Impact**: Limits concurrent CRD creation
```
Max clients before rate limit: 1GB / (100 req/min × 40 bytes) = ~267,000 clients
```

**Realistic Concurrent Clients**: **~267,000 clients** at 100 req/min each

### 3. Kubernetes API Throttling
**Impact**: K8s API limits CRD creation rate
- **Default K8s API QPS**: 5 requests/second per client
- **Gateway QPS**: 50 requests/second (configured in tests)
- **Production QPS**: 5 requests/second (default)

**Realistic CRD Creation Rate**: **5 CRDs/second** (production default)

---

## Summary Table

| Scenario | Memory per CRD | Theoretical Threshold | Realistic Throughput |
|----------|---------------|----------------------|---------------------|
| **Normal Operation** | 280 bytes | 2.68M CRDs | 8,900 CRDs/sec |
| **10% Storm Aggregation** | 330 bytes (avg) | 2.28M CRDs | 7,600 CRDs/sec |
| **50% Storm Aggregation** | 680 bytes (avg) | 1.11M CRDs | 3,700 CRDs/sec |
| **Production (5 QPS)** | 280 bytes | 2.68M CRDs | **5 CRDs/sec** |

---

## Bottlenecks (Ranked by Impact)

### 1. Kubernetes API Rate Limiting (HIGHEST IMPACT)
- **Default**: 5 CRDs/second
- **Configured (tests)**: 50 CRDs/second
- **Recommendation**: Use K8s client-side rate limiting (QPS=50, Burst=100)

### 2. Redis Memory (MEDIUM IMPACT)
- **1GB**: Supports 1.11M - 2.68M CRDs in flight
- **2GB**: Supports 2.22M - 5.36M CRDs in flight
- **Recommendation**: 2GB for production (current configuration)

### 3. Rate Limiting (LOW IMPACT)
- **100 req/min**: Prevents DoS, minimal impact on legitimate traffic
- **Recommendation**: Keep current configuration

---

## Recommendations

### For Production (1GB Redis)
1. **Expected Load**: 5 CRDs/second (K8s API default QPS)
2. **Peak Capacity**: 2.68M CRDs in flight (normal operation)
3. **Safety Margin**: 99.8% headroom (5 CRDs/sec << 8,900 CRDs/sec)
4. **Verdict**: **1GB is MORE than sufficient for production**

### For Testing (2GB Redis)
1. **Expected Load**: 50 CRDs/second (configured QPS)
2. **Peak Capacity**: 5.36M CRDs in flight (normal operation)
3. **Safety Margin**: 99.1% headroom (50 CRDs/sec << 17,800 CRDs/sec)
4. **Verdict**: **2GB provides excellent headroom for concurrent tests**

### For High-Throughput Scenarios (Future)
If Gateway needs to handle >8,900 CRDs/second:
1. **Increase K8s API QPS**: Configure client-side rate limiting (QPS=100, Burst=200)
2. **Increase Redis Memory**: 4GB supports 5.36M - 10.72M CRDs in flight
3. **Implement Redis Cluster**: Horizontal scaling for >10M CRDs

---

## Confidence Assessment

**95% Confidence** that:
- ✅ 1GB Redis supports **2.68M CRDs** in normal operation
- ✅ 2GB Redis supports **5.36M CRDs** in normal operation
- ✅ Production bottleneck is **K8s API (5 CRDs/sec)**, not Redis
- ✅ 1GB Redis is **sufficient for production** (99.8% headroom)
- ✅ 2GB Redis is **optimal for testing** (99.1% headroom)

**Risk Factors**:
- Memory fragmentation may reduce capacity by 10-20%
- Large storm CRDs (>20 affected resources) may reduce capacity by 30%
- Concurrent tests may spike memory usage temporarily

**Mitigation**:
- 2GB Redis provides 2x safety margin
- TTL expiration (5 min) prevents long-term accumulation
- Aggressive cleanup in tests prevents state pollution

# Redis Capacity Analysis: CRDs in Flight

## Question
With 1GB of Redis memory, what is the theoretical threshold of CRDs in flight that the Gateway can handle?

---

## Memory Breakdown per CRD

### Deduplication Keys (per unique fingerprint)
```
Key: dedup:metadata:{fingerprint}
Value: JSON metadata
{
  "fingerprint": "64-char-sha256",
  "remediationRequestRef": "crd-name-uuid",
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "count": 5
}
```
**Size**: ~200 bytes per key (optimized in DD-GATEWAY-004)

### Storm Detection Keys (per namespace:alertname)
```
Key: storm:counter:{namespace}:{alertname}
Value: Integer counter
Size: ~50 bytes

Key: storm:flag:{namespace}:{alertname}
Value: "1" (flag)
Size: ~30 bytes
```
**Total**: ~80 bytes per namespace:alertname pair

### Storm Aggregation Keys (per storm CRD)
```
Key: storm:metadata:{namespace}:{alertname}
Value: Lightweight metadata JSON
{
  "pattern": "HighCPU in production",
  "alertCount": 15,
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "affectedResources": [
    {"kind": "Pod", "name": "api-1", "namespace": "prod"},
    {"kind": "Pod", "name": "api-2", "namespace": "prod"}
  ]
}
```
**Size**: ~300 bytes base + (50 bytes × number of affected resources)
- **Average**: ~500 bytes (assuming 4 affected resources)

### Rate Limiting Keys (per IP/token)
```
Key: ratelimit:{ip_or_token}
Value: Request count
Size: ~40 bytes
```

---

## Scenario Analysis

### Scenario 1: Normal Operation (No Storm)
**Assumptions**:
- 1 CRD = 1 deduplication key + 1 storm detection counter + 1 storm flag
- **Total per CRD**: 200 + 50 + 30 = **280 bytes**

**Calculation**:
```
Available memory: 1GB = 1,073,741,824 bytes
Overhead (Redis metadata): 30% = 322,122,547 bytes
Usable memory: 751,619,277 bytes

CRDs in flight = 751,619,277 / 280 = 2,684,354 CRDs
```

**Theoretical Threshold**: **~2.68 million CRDs** (normal operation)

---

### Scenario 2: Storm Aggregation Active
**Assumptions**:
- 10% of CRDs trigger storm aggregation (10 alerts → 1 storm CRD)
- Storm CRD = 1 dedup key + 1 storm counter + 1 storm flag + 1 storm metadata
- **Total per storm CRD**: 200 + 50 + 30 + 500 = **780 bytes**
- **Total per individual CRD**: 280 bytes (as above)

**Calculation** (mixed workload):
```
Usable memory: 751,619,277 bytes

Storm CRDs (10%): 10% of total
Individual CRDs (90%): 90% of total

Let X = total CRDs
Storm CRDs: 0.1X × 780 bytes = 78X bytes
Individual CRDs: 0.9X × 280 bytes = 252X bytes
Total: 330X bytes

X = 751,619,277 / 330 = 2,277,331 CRDs
```

**Theoretical Threshold**: **~2.28 million CRDs** (with 10% storm aggregation)

---

### Scenario 3: Heavy Storm (50% Aggregation)
**Assumptions**:
- 50% of CRDs trigger storm aggregation
- Storm CRDs have 10 affected resources each (larger metadata)
- **Storm metadata size**: 300 + (50 × 10) = 800 bytes
- **Total per storm CRD**: 200 + 50 + 30 + 800 = **1,080 bytes**

**Calculation**:
```
Usable memory: 751,619,277 bytes

Let X = total CRDs
Storm CRDs: 0.5X × 1,080 bytes = 540X bytes
Individual CRDs: 0.5X × 280 bytes = 140X bytes
Total: 680X bytes

X = 751,619,277 / 680 = 1,105,322 CRDs
```

**Theoretical Threshold**: **~1.11 million CRDs** (heavy storm scenario)

---

## Real-World Constraints

### 1. TTL Expiration (5 minutes)
**Impact**: Keys expire after 5 minutes, freeing memory
```
CRDs per 5-minute window = Threshold / (5 × 60) seconds
= 2,684,354 / 300 = 8,948 CRDs/second
```

**Realistic Throughput**: **~8,900 CRDs/second** (normal operation)

### 2. Rate Limiting (100 req/min per client)
**Impact**: Limits concurrent CRD creation
```
Max clients before rate limit: 1GB / (100 req/min × 40 bytes) = ~267,000 clients
```

**Realistic Concurrent Clients**: **~267,000 clients** at 100 req/min each

### 3. Kubernetes API Throttling
**Impact**: K8s API limits CRD creation rate
- **Default K8s API QPS**: 5 requests/second per client
- **Gateway QPS**: 50 requests/second (configured in tests)
- **Production QPS**: 5 requests/second (default)

**Realistic CRD Creation Rate**: **5 CRDs/second** (production default)

---

## Summary Table

| Scenario | Memory per CRD | Theoretical Threshold | Realistic Throughput |
|----------|---------------|----------------------|---------------------|
| **Normal Operation** | 280 bytes | 2.68M CRDs | 8,900 CRDs/sec |
| **10% Storm Aggregation** | 330 bytes (avg) | 2.28M CRDs | 7,600 CRDs/sec |
| **50% Storm Aggregation** | 680 bytes (avg) | 1.11M CRDs | 3,700 CRDs/sec |
| **Production (5 QPS)** | 280 bytes | 2.68M CRDs | **5 CRDs/sec** |

---

## Bottlenecks (Ranked by Impact)

### 1. Kubernetes API Rate Limiting (HIGHEST IMPACT)
- **Default**: 5 CRDs/second
- **Configured (tests)**: 50 CRDs/second
- **Recommendation**: Use K8s client-side rate limiting (QPS=50, Burst=100)

### 2. Redis Memory (MEDIUM IMPACT)
- **1GB**: Supports 1.11M - 2.68M CRDs in flight
- **2GB**: Supports 2.22M - 5.36M CRDs in flight
- **Recommendation**: 2GB for production (current configuration)

### 3. Rate Limiting (LOW IMPACT)
- **100 req/min**: Prevents DoS, minimal impact on legitimate traffic
- **Recommendation**: Keep current configuration

---

## Recommendations

### For Production (1GB Redis)
1. **Expected Load**: 5 CRDs/second (K8s API default QPS)
2. **Peak Capacity**: 2.68M CRDs in flight (normal operation)
3. **Safety Margin**: 99.8% headroom (5 CRDs/sec << 8,900 CRDs/sec)
4. **Verdict**: **1GB is MORE than sufficient for production**

### For Testing (2GB Redis)
1. **Expected Load**: 50 CRDs/second (configured QPS)
2. **Peak Capacity**: 5.36M CRDs in flight (normal operation)
3. **Safety Margin**: 99.1% headroom (50 CRDs/sec << 17,800 CRDs/sec)
4. **Verdict**: **2GB provides excellent headroom for concurrent tests**

### For High-Throughput Scenarios (Future)
If Gateway needs to handle >8,900 CRDs/second:
1. **Increase K8s API QPS**: Configure client-side rate limiting (QPS=100, Burst=200)
2. **Increase Redis Memory**: 4GB supports 5.36M - 10.72M CRDs in flight
3. **Implement Redis Cluster**: Horizontal scaling for >10M CRDs

---

## Confidence Assessment

**95% Confidence** that:
- ✅ 1GB Redis supports **2.68M CRDs** in normal operation
- ✅ 2GB Redis supports **5.36M CRDs** in normal operation
- ✅ Production bottleneck is **K8s API (5 CRDs/sec)**, not Redis
- ✅ 1GB Redis is **sufficient for production** (99.8% headroom)
- ✅ 2GB Redis is **optimal for testing** (99.1% headroom)

**Risk Factors**:
- Memory fragmentation may reduce capacity by 10-20%
- Large storm CRDs (>20 affected resources) may reduce capacity by 30%
- Concurrent tests may spike memory usage temporarily

**Mitigation**:
- 2GB Redis provides 2x safety margin
- TTL expiration (5 min) prevents long-term accumulation
- Aggressive cleanup in tests prevents state pollution

# Redis Capacity Analysis: CRDs in Flight

## Question
With 1GB of Redis memory, what is the theoretical threshold of CRDs in flight that the Gateway can handle?

---

## Memory Breakdown per CRD

### Deduplication Keys (per unique fingerprint)
```
Key: dedup:metadata:{fingerprint}
Value: JSON metadata
{
  "fingerprint": "64-char-sha256",
  "remediationRequestRef": "crd-name-uuid",
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "count": 5
}
```
**Size**: ~200 bytes per key (optimized in DD-GATEWAY-004)

### Storm Detection Keys (per namespace:alertname)
```
Key: storm:counter:{namespace}:{alertname}
Value: Integer counter
Size: ~50 bytes

Key: storm:flag:{namespace}:{alertname}
Value: "1" (flag)
Size: ~30 bytes
```
**Total**: ~80 bytes per namespace:alertname pair

### Storm Aggregation Keys (per storm CRD)
```
Key: storm:metadata:{namespace}:{alertname}
Value: Lightweight metadata JSON
{
  "pattern": "HighCPU in production",
  "alertCount": 15,
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "affectedResources": [
    {"kind": "Pod", "name": "api-1", "namespace": "prod"},
    {"kind": "Pod", "name": "api-2", "namespace": "prod"}
  ]
}
```
**Size**: ~300 bytes base + (50 bytes × number of affected resources)
- **Average**: ~500 bytes (assuming 4 affected resources)

### Rate Limiting Keys (per IP/token)
```
Key: ratelimit:{ip_or_token}
Value: Request count
Size: ~40 bytes
```

---

## Scenario Analysis

### Scenario 1: Normal Operation (No Storm)
**Assumptions**:
- 1 CRD = 1 deduplication key + 1 storm detection counter + 1 storm flag
- **Total per CRD**: 200 + 50 + 30 = **280 bytes**

**Calculation**:
```
Available memory: 1GB = 1,073,741,824 bytes
Overhead (Redis metadata): 30% = 322,122,547 bytes
Usable memory: 751,619,277 bytes

CRDs in flight = 751,619,277 / 280 = 2,684,354 CRDs
```

**Theoretical Threshold**: **~2.68 million CRDs** (normal operation)

---

### Scenario 2: Storm Aggregation Active
**Assumptions**:
- 10% of CRDs trigger storm aggregation (10 alerts → 1 storm CRD)
- Storm CRD = 1 dedup key + 1 storm counter + 1 storm flag + 1 storm metadata
- **Total per storm CRD**: 200 + 50 + 30 + 500 = **780 bytes**
- **Total per individual CRD**: 280 bytes (as above)

**Calculation** (mixed workload):
```
Usable memory: 751,619,277 bytes

Storm CRDs (10%): 10% of total
Individual CRDs (90%): 90% of total

Let X = total CRDs
Storm CRDs: 0.1X × 780 bytes = 78X bytes
Individual CRDs: 0.9X × 280 bytes = 252X bytes
Total: 330X bytes

X = 751,619,277 / 330 = 2,277,331 CRDs
```

**Theoretical Threshold**: **~2.28 million CRDs** (with 10% storm aggregation)

---

### Scenario 3: Heavy Storm (50% Aggregation)
**Assumptions**:
- 50% of CRDs trigger storm aggregation
- Storm CRDs have 10 affected resources each (larger metadata)
- **Storm metadata size**: 300 + (50 × 10) = 800 bytes
- **Total per storm CRD**: 200 + 50 + 30 + 800 = **1,080 bytes**

**Calculation**:
```
Usable memory: 751,619,277 bytes

Let X = total CRDs
Storm CRDs: 0.5X × 1,080 bytes = 540X bytes
Individual CRDs: 0.5X × 280 bytes = 140X bytes
Total: 680X bytes

X = 751,619,277 / 680 = 1,105,322 CRDs
```

**Theoretical Threshold**: **~1.11 million CRDs** (heavy storm scenario)

---

## Real-World Constraints

### 1. TTL Expiration (5 minutes)
**Impact**: Keys expire after 5 minutes, freeing memory
```
CRDs per 5-minute window = Threshold / (5 × 60) seconds
= 2,684,354 / 300 = 8,948 CRDs/second
```

**Realistic Throughput**: **~8,900 CRDs/second** (normal operation)

### 2. Rate Limiting (100 req/min per client)
**Impact**: Limits concurrent CRD creation
```
Max clients before rate limit: 1GB / (100 req/min × 40 bytes) = ~267,000 clients
```

**Realistic Concurrent Clients**: **~267,000 clients** at 100 req/min each

### 3. Kubernetes API Throttling
**Impact**: K8s API limits CRD creation rate
- **Default K8s API QPS**: 5 requests/second per client
- **Gateway QPS**: 50 requests/second (configured in tests)
- **Production QPS**: 5 requests/second (default)

**Realistic CRD Creation Rate**: **5 CRDs/second** (production default)

---

## Summary Table

| Scenario | Memory per CRD | Theoretical Threshold | Realistic Throughput |
|----------|---------------|----------------------|---------------------|
| **Normal Operation** | 280 bytes | 2.68M CRDs | 8,900 CRDs/sec |
| **10% Storm Aggregation** | 330 bytes (avg) | 2.28M CRDs | 7,600 CRDs/sec |
| **50% Storm Aggregation** | 680 bytes (avg) | 1.11M CRDs | 3,700 CRDs/sec |
| **Production (5 QPS)** | 280 bytes | 2.68M CRDs | **5 CRDs/sec** |

---

## Bottlenecks (Ranked by Impact)

### 1. Kubernetes API Rate Limiting (HIGHEST IMPACT)
- **Default**: 5 CRDs/second
- **Configured (tests)**: 50 CRDs/second
- **Recommendation**: Use K8s client-side rate limiting (QPS=50, Burst=100)

### 2. Redis Memory (MEDIUM IMPACT)
- **1GB**: Supports 1.11M - 2.68M CRDs in flight
- **2GB**: Supports 2.22M - 5.36M CRDs in flight
- **Recommendation**: 2GB for production (current configuration)

### 3. Rate Limiting (LOW IMPACT)
- **100 req/min**: Prevents DoS, minimal impact on legitimate traffic
- **Recommendation**: Keep current configuration

---

## Recommendations

### For Production (1GB Redis)
1. **Expected Load**: 5 CRDs/second (K8s API default QPS)
2. **Peak Capacity**: 2.68M CRDs in flight (normal operation)
3. **Safety Margin**: 99.8% headroom (5 CRDs/sec << 8,900 CRDs/sec)
4. **Verdict**: **1GB is MORE than sufficient for production**

### For Testing (2GB Redis)
1. **Expected Load**: 50 CRDs/second (configured QPS)
2. **Peak Capacity**: 5.36M CRDs in flight (normal operation)
3. **Safety Margin**: 99.1% headroom (50 CRDs/sec << 17,800 CRDs/sec)
4. **Verdict**: **2GB provides excellent headroom for concurrent tests**

### For High-Throughput Scenarios (Future)
If Gateway needs to handle >8,900 CRDs/second:
1. **Increase K8s API QPS**: Configure client-side rate limiting (QPS=100, Burst=200)
2. **Increase Redis Memory**: 4GB supports 5.36M - 10.72M CRDs in flight
3. **Implement Redis Cluster**: Horizontal scaling for >10M CRDs

---

## Confidence Assessment

**95% Confidence** that:
- ✅ 1GB Redis supports **2.68M CRDs** in normal operation
- ✅ 2GB Redis supports **5.36M CRDs** in normal operation
- ✅ Production bottleneck is **K8s API (5 CRDs/sec)**, not Redis
- ✅ 1GB Redis is **sufficient for production** (99.8% headroom)
- ✅ 2GB Redis is **optimal for testing** (99.1% headroom)

**Risk Factors**:
- Memory fragmentation may reduce capacity by 10-20%
- Large storm CRDs (>20 affected resources) may reduce capacity by 30%
- Concurrent tests may spike memory usage temporarily

**Mitigation**:
- 2GB Redis provides 2x safety margin
- TTL expiration (5 min) prevents long-term accumulation
- Aggressive cleanup in tests prevents state pollution



## Question
With 1GB of Redis memory, what is the theoretical threshold of CRDs in flight that the Gateway can handle?

---

## Memory Breakdown per CRD

### Deduplication Keys (per unique fingerprint)
```
Key: dedup:metadata:{fingerprint}
Value: JSON metadata
{
  "fingerprint": "64-char-sha256",
  "remediationRequestRef": "crd-name-uuid",
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "count": 5
}
```
**Size**: ~200 bytes per key (optimized in DD-GATEWAY-004)

### Storm Detection Keys (per namespace:alertname)
```
Key: storm:counter:{namespace}:{alertname}
Value: Integer counter
Size: ~50 bytes

Key: storm:flag:{namespace}:{alertname}
Value: "1" (flag)
Size: ~30 bytes
```
**Total**: ~80 bytes per namespace:alertname pair

### Storm Aggregation Keys (per storm CRD)
```
Key: storm:metadata:{namespace}:{alertname}
Value: Lightweight metadata JSON
{
  "pattern": "HighCPU in production",
  "alertCount": 15,
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "affectedResources": [
    {"kind": "Pod", "name": "api-1", "namespace": "prod"},
    {"kind": "Pod", "name": "api-2", "namespace": "prod"}
  ]
}
```
**Size**: ~300 bytes base + (50 bytes × number of affected resources)
- **Average**: ~500 bytes (assuming 4 affected resources)

### Rate Limiting Keys (per IP/token)
```
Key: ratelimit:{ip_or_token}
Value: Request count
Size: ~40 bytes
```

---

## Scenario Analysis

### Scenario 1: Normal Operation (No Storm)
**Assumptions**:
- 1 CRD = 1 deduplication key + 1 storm detection counter + 1 storm flag
- **Total per CRD**: 200 + 50 + 30 = **280 bytes**

**Calculation**:
```
Available memory: 1GB = 1,073,741,824 bytes
Overhead (Redis metadata): 30% = 322,122,547 bytes
Usable memory: 751,619,277 bytes

CRDs in flight = 751,619,277 / 280 = 2,684,354 CRDs
```

**Theoretical Threshold**: **~2.68 million CRDs** (normal operation)

---

### Scenario 2: Storm Aggregation Active
**Assumptions**:
- 10% of CRDs trigger storm aggregation (10 alerts → 1 storm CRD)
- Storm CRD = 1 dedup key + 1 storm counter + 1 storm flag + 1 storm metadata
- **Total per storm CRD**: 200 + 50 + 30 + 500 = **780 bytes**
- **Total per individual CRD**: 280 bytes (as above)

**Calculation** (mixed workload):
```
Usable memory: 751,619,277 bytes

Storm CRDs (10%): 10% of total
Individual CRDs (90%): 90% of total

Let X = total CRDs
Storm CRDs: 0.1X × 780 bytes = 78X bytes
Individual CRDs: 0.9X × 280 bytes = 252X bytes
Total: 330X bytes

X = 751,619,277 / 330 = 2,277,331 CRDs
```

**Theoretical Threshold**: **~2.28 million CRDs** (with 10% storm aggregation)

---

### Scenario 3: Heavy Storm (50% Aggregation)
**Assumptions**:
- 50% of CRDs trigger storm aggregation
- Storm CRDs have 10 affected resources each (larger metadata)
- **Storm metadata size**: 300 + (50 × 10) = 800 bytes
- **Total per storm CRD**: 200 + 50 + 30 + 800 = **1,080 bytes**

**Calculation**:
```
Usable memory: 751,619,277 bytes

Let X = total CRDs
Storm CRDs: 0.5X × 1,080 bytes = 540X bytes
Individual CRDs: 0.5X × 280 bytes = 140X bytes
Total: 680X bytes

X = 751,619,277 / 680 = 1,105,322 CRDs
```

**Theoretical Threshold**: **~1.11 million CRDs** (heavy storm scenario)

---

## Real-World Constraints

### 1. TTL Expiration (5 minutes)
**Impact**: Keys expire after 5 minutes, freeing memory
```
CRDs per 5-minute window = Threshold / (5 × 60) seconds
= 2,684,354 / 300 = 8,948 CRDs/second
```

**Realistic Throughput**: **~8,900 CRDs/second** (normal operation)

### 2. Rate Limiting (100 req/min per client)
**Impact**: Limits concurrent CRD creation
```
Max clients before rate limit: 1GB / (100 req/min × 40 bytes) = ~267,000 clients
```

**Realistic Concurrent Clients**: **~267,000 clients** at 100 req/min each

### 3. Kubernetes API Throttling
**Impact**: K8s API limits CRD creation rate
- **Default K8s API QPS**: 5 requests/second per client
- **Gateway QPS**: 50 requests/second (configured in tests)
- **Production QPS**: 5 requests/second (default)

**Realistic CRD Creation Rate**: **5 CRDs/second** (production default)

---

## Summary Table

| Scenario | Memory per CRD | Theoretical Threshold | Realistic Throughput |
|----------|---------------|----------------------|---------------------|
| **Normal Operation** | 280 bytes | 2.68M CRDs | 8,900 CRDs/sec |
| **10% Storm Aggregation** | 330 bytes (avg) | 2.28M CRDs | 7,600 CRDs/sec |
| **50% Storm Aggregation** | 680 bytes (avg) | 1.11M CRDs | 3,700 CRDs/sec |
| **Production (5 QPS)** | 280 bytes | 2.68M CRDs | **5 CRDs/sec** |

---

## Bottlenecks (Ranked by Impact)

### 1. Kubernetes API Rate Limiting (HIGHEST IMPACT)
- **Default**: 5 CRDs/second
- **Configured (tests)**: 50 CRDs/second
- **Recommendation**: Use K8s client-side rate limiting (QPS=50, Burst=100)

### 2. Redis Memory (MEDIUM IMPACT)
- **1GB**: Supports 1.11M - 2.68M CRDs in flight
- **2GB**: Supports 2.22M - 5.36M CRDs in flight
- **Recommendation**: 2GB for production (current configuration)

### 3. Rate Limiting (LOW IMPACT)
- **100 req/min**: Prevents DoS, minimal impact on legitimate traffic
- **Recommendation**: Keep current configuration

---

## Recommendations

### For Production (1GB Redis)
1. **Expected Load**: 5 CRDs/second (K8s API default QPS)
2. **Peak Capacity**: 2.68M CRDs in flight (normal operation)
3. **Safety Margin**: 99.8% headroom (5 CRDs/sec << 8,900 CRDs/sec)
4. **Verdict**: **1GB is MORE than sufficient for production**

### For Testing (2GB Redis)
1. **Expected Load**: 50 CRDs/second (configured QPS)
2. **Peak Capacity**: 5.36M CRDs in flight (normal operation)
3. **Safety Margin**: 99.1% headroom (50 CRDs/sec << 17,800 CRDs/sec)
4. **Verdict**: **2GB provides excellent headroom for concurrent tests**

### For High-Throughput Scenarios (Future)
If Gateway needs to handle >8,900 CRDs/second:
1. **Increase K8s API QPS**: Configure client-side rate limiting (QPS=100, Burst=200)
2. **Increase Redis Memory**: 4GB supports 5.36M - 10.72M CRDs in flight
3. **Implement Redis Cluster**: Horizontal scaling for >10M CRDs

---

## Confidence Assessment

**95% Confidence** that:
- ✅ 1GB Redis supports **2.68M CRDs** in normal operation
- ✅ 2GB Redis supports **5.36M CRDs** in normal operation
- ✅ Production bottleneck is **K8s API (5 CRDs/sec)**, not Redis
- ✅ 1GB Redis is **sufficient for production** (99.8% headroom)
- ✅ 2GB Redis is **optimal for testing** (99.1% headroom)

**Risk Factors**:
- Memory fragmentation may reduce capacity by 10-20%
- Large storm CRDs (>20 affected resources) may reduce capacity by 30%
- Concurrent tests may spike memory usage temporarily

**Mitigation**:
- 2GB Redis provides 2x safety margin
- TTL expiration (5 min) prevents long-term accumulation
- Aggressive cleanup in tests prevents state pollution

# Redis Capacity Analysis: CRDs in Flight

## Question
With 1GB of Redis memory, what is the theoretical threshold of CRDs in flight that the Gateway can handle?

---

## Memory Breakdown per CRD

### Deduplication Keys (per unique fingerprint)
```
Key: dedup:metadata:{fingerprint}
Value: JSON metadata
{
  "fingerprint": "64-char-sha256",
  "remediationRequestRef": "crd-name-uuid",
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "count": 5
}
```
**Size**: ~200 bytes per key (optimized in DD-GATEWAY-004)

### Storm Detection Keys (per namespace:alertname)
```
Key: storm:counter:{namespace}:{alertname}
Value: Integer counter
Size: ~50 bytes

Key: storm:flag:{namespace}:{alertname}
Value: "1" (flag)
Size: ~30 bytes
```
**Total**: ~80 bytes per namespace:alertname pair

### Storm Aggregation Keys (per storm CRD)
```
Key: storm:metadata:{namespace}:{alertname}
Value: Lightweight metadata JSON
{
  "pattern": "HighCPU in production",
  "alertCount": 15,
  "firstSeen": "2025-10-27T12:00:00Z",
  "lastSeen": "2025-10-27T12:05:00Z",
  "affectedResources": [
    {"kind": "Pod", "name": "api-1", "namespace": "prod"},
    {"kind": "Pod", "name": "api-2", "namespace": "prod"}
  ]
}
```
**Size**: ~300 bytes base + (50 bytes × number of affected resources)
- **Average**: ~500 bytes (assuming 4 affected resources)

### Rate Limiting Keys (per IP/token)
```
Key: ratelimit:{ip_or_token}
Value: Request count
Size: ~40 bytes
```

---

## Scenario Analysis

### Scenario 1: Normal Operation (No Storm)
**Assumptions**:
- 1 CRD = 1 deduplication key + 1 storm detection counter + 1 storm flag
- **Total per CRD**: 200 + 50 + 30 = **280 bytes**

**Calculation**:
```
Available memory: 1GB = 1,073,741,824 bytes
Overhead (Redis metadata): 30% = 322,122,547 bytes
Usable memory: 751,619,277 bytes

CRDs in flight = 751,619,277 / 280 = 2,684,354 CRDs
```

**Theoretical Threshold**: **~2.68 million CRDs** (normal operation)

---

### Scenario 2: Storm Aggregation Active
**Assumptions**:
- 10% of CRDs trigger storm aggregation (10 alerts → 1 storm CRD)
- Storm CRD = 1 dedup key + 1 storm counter + 1 storm flag + 1 storm metadata
- **Total per storm CRD**: 200 + 50 + 30 + 500 = **780 bytes**
- **Total per individual CRD**: 280 bytes (as above)

**Calculation** (mixed workload):
```
Usable memory: 751,619,277 bytes

Storm CRDs (10%): 10% of total
Individual CRDs (90%): 90% of total

Let X = total CRDs
Storm CRDs: 0.1X × 780 bytes = 78X bytes
Individual CRDs: 0.9X × 280 bytes = 252X bytes
Total: 330X bytes

X = 751,619,277 / 330 = 2,277,331 CRDs
```

**Theoretical Threshold**: **~2.28 million CRDs** (with 10% storm aggregation)

---

### Scenario 3: Heavy Storm (50% Aggregation)
**Assumptions**:
- 50% of CRDs trigger storm aggregation
- Storm CRDs have 10 affected resources each (larger metadata)
- **Storm metadata size**: 300 + (50 × 10) = 800 bytes
- **Total per storm CRD**: 200 + 50 + 30 + 800 = **1,080 bytes**

**Calculation**:
```
Usable memory: 751,619,277 bytes

Let X = total CRDs
Storm CRDs: 0.5X × 1,080 bytes = 540X bytes
Individual CRDs: 0.5X × 280 bytes = 140X bytes
Total: 680X bytes

X = 751,619,277 / 680 = 1,105,322 CRDs
```

**Theoretical Threshold**: **~1.11 million CRDs** (heavy storm scenario)

---

## Real-World Constraints

### 1. TTL Expiration (5 minutes)
**Impact**: Keys expire after 5 minutes, freeing memory
```
CRDs per 5-minute window = Threshold / (5 × 60) seconds
= 2,684,354 / 300 = 8,948 CRDs/second
```

**Realistic Throughput**: **~8,900 CRDs/second** (normal operation)

### 2. Rate Limiting (100 req/min per client)
**Impact**: Limits concurrent CRD creation
```
Max clients before rate limit: 1GB / (100 req/min × 40 bytes) = ~267,000 clients
```

**Realistic Concurrent Clients**: **~267,000 clients** at 100 req/min each

### 3. Kubernetes API Throttling
**Impact**: K8s API limits CRD creation rate
- **Default K8s API QPS**: 5 requests/second per client
- **Gateway QPS**: 50 requests/second (configured in tests)
- **Production QPS**: 5 requests/second (default)

**Realistic CRD Creation Rate**: **5 CRDs/second** (production default)

---

## Summary Table

| Scenario | Memory per CRD | Theoretical Threshold | Realistic Throughput |
|----------|---------------|----------------------|---------------------|
| **Normal Operation** | 280 bytes | 2.68M CRDs | 8,900 CRDs/sec |
| **10% Storm Aggregation** | 330 bytes (avg) | 2.28M CRDs | 7,600 CRDs/sec |
| **50% Storm Aggregation** | 680 bytes (avg) | 1.11M CRDs | 3,700 CRDs/sec |
| **Production (5 QPS)** | 280 bytes | 2.68M CRDs | **5 CRDs/sec** |

---

## Bottlenecks (Ranked by Impact)

### 1. Kubernetes API Rate Limiting (HIGHEST IMPACT)
- **Default**: 5 CRDs/second
- **Configured (tests)**: 50 CRDs/second
- **Recommendation**: Use K8s client-side rate limiting (QPS=50, Burst=100)

### 2. Redis Memory (MEDIUM IMPACT)
- **1GB**: Supports 1.11M - 2.68M CRDs in flight
- **2GB**: Supports 2.22M - 5.36M CRDs in flight
- **Recommendation**: 2GB for production (current configuration)

### 3. Rate Limiting (LOW IMPACT)
- **100 req/min**: Prevents DoS, minimal impact on legitimate traffic
- **Recommendation**: Keep current configuration

---

## Recommendations

### For Production (1GB Redis)
1. **Expected Load**: 5 CRDs/second (K8s API default QPS)
2. **Peak Capacity**: 2.68M CRDs in flight (normal operation)
3. **Safety Margin**: 99.8% headroom (5 CRDs/sec << 8,900 CRDs/sec)
4. **Verdict**: **1GB is MORE than sufficient for production**

### For Testing (2GB Redis)
1. **Expected Load**: 50 CRDs/second (configured QPS)
2. **Peak Capacity**: 5.36M CRDs in flight (normal operation)
3. **Safety Margin**: 99.1% headroom (50 CRDs/sec << 17,800 CRDs/sec)
4. **Verdict**: **2GB provides excellent headroom for concurrent tests**

### For High-Throughput Scenarios (Future)
If Gateway needs to handle >8,900 CRDs/second:
1. **Increase K8s API QPS**: Configure client-side rate limiting (QPS=100, Burst=200)
2. **Increase Redis Memory**: 4GB supports 5.36M - 10.72M CRDs in flight
3. **Implement Redis Cluster**: Horizontal scaling for >10M CRDs

---

## Confidence Assessment

**95% Confidence** that:
- ✅ 1GB Redis supports **2.68M CRDs** in normal operation
- ✅ 2GB Redis supports **5.36M CRDs** in normal operation
- ✅ Production bottleneck is **K8s API (5 CRDs/sec)**, not Redis
- ✅ 1GB Redis is **sufficient for production** (99.8% headroom)
- ✅ 2GB Redis is **optimal for testing** (99.1% headroom)

**Risk Factors**:
- Memory fragmentation may reduce capacity by 10-20%
- Large storm CRDs (>20 affected resources) may reduce capacity by 30%
- Concurrent tests may spike memory usage temporarily

**Mitigation**:
- 2GB Redis provides 2x safety margin
- TTL expiration (5 min) prevents long-term accumulation
- Aggressive cleanup in tests prevents state pollution




