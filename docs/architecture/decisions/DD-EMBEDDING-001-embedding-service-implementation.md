# DD-EMBEDDING-001: Embedding Service Implementation Strategy

**Date**: November 14, 2025
**Status**: ✅ **APPROVED** (Option A - Python Microservice)
**Decision Maker**: Kubernaut Architecture Team
**Authority**: BR-STORAGE-012 (Playbook Semantic Search), DD-STORAGE-008 (Playbook Schema)
**Affects**: Data Storage Service V1.0, Playbook Semantic Search
**Version**: 1.0

---

## 📋 **Status**

**✅ APPROVED** (2025-11-14)
**Last Reviewed**: 2025-11-14
**Confidence**: 90%

---

## 🎯 **Context & Problem**

### **Problem Statement**

The Data Storage Service requires vector embedding generation for playbook semantic search (BR-STORAGE-012). We need to convert playbook text content into 384-dimensional vectors using the sentence-transformers model for similarity-based retrieval.

**Key Requirements**:
1. Generate 384-dimensional embeddings from playbook text
2. Support sentence-transformers model (`all-MiniLM-L6-v2`)
3. Integrate with Data Storage Service REST API
4. Meet < 2.5s total latency budget for semantic search
5. Support future model upgrades and experimentation

### **Current State**

- ✅ **Interface defined**: `pkg/datastorage/embedding/interfaces.go` (`EmbeddingAPIClient`)
- ✅ **Model selected**: sentence-transformers `all-MiniLM-L6-v2` (384 dimensions)
- ✅ **Use case validated**: DD-STORAGE-012 PoC shows effective playbook matching
- ❌ **NO implementation**: Only interface exists, no concrete service

### **Decision Scope**

Choose the implementation strategy for the embedding service that best fits Kubernaut's:
- Microservices architecture
- Development velocity requirements
- Operational simplicity goals
- Future ML experimentation needs

---

## 🔍 **Alternatives Considered**

### **Alternative 1: External API (HuggingFace Inference API)**

**Approach**: Use HuggingFace's hosted inference API for embedding generation.

**Architecture**:
```
Data Storage Service
    ↓ HTTPS
HuggingFace Inference API
    ↓ 384-dim vector
Data Storage Service
```

**Pros**:
- ✅ **Zero infrastructure**: No service to deploy or maintain
- ✅ **Always up-to-date**: HuggingFace manages model updates
- ✅ **Instant scaling**: HuggingFace handles load
- ✅ **Fastest development**: Just HTTP client integration

**Cons**:
- ❌ **External dependency**: Internet connectivity required
- ❌ **Latency**: 200-500ms per request (network + API processing)
- ❌ **Cost**: $0.0004 per 1K tokens (~$0.40 per 1M tokens)
- ❌ **Rate limits**: Free tier: 1K requests/day; Pro: 10K requests/day
- ❌ **Data privacy**: Playbook content sent to external service
- ❌ **No air-gapped support**: Cannot run in isolated environments

**Confidence**: 30% (rejected - external dependency unacceptable)

---

### **Alternative 2: Go + cgo Bindings (ONNX Runtime)**

**Approach**: Embed ONNX Runtime in Go binary using cgo, load sentence-transformers model as ONNX format.

**Architecture**:
```
Data Storage Service (Go)
    ↓ cgo call
ONNX Runtime (C++)
    ↓ model inference
384-dim vector
```

**Pros**:
- ✅ **Single binary**: No separate service to deploy
- ✅ **Low latency**: In-process call (~10-50ms)
- ✅ **No network overhead**: Direct function call
- ✅ **Smaller footprint**: Single Go binary (~50MB)

**Cons**:
- ❌ **cgo complexity**: Cross-compilation nightmares (Linux/macOS/Windows)
- ❌ **Build complexity**: Requires ONNX Runtime, C++ dependencies, protobuf
- ❌ **Debugging hell**: Mixed Go/C++ stack traces, memory leaks
- ❌ **Limited flexibility**: Hard to swap models (requires ONNX conversion)
- ❌ **Maintenance burden**: cgo bindings break with Go version upgrades
- ❌ **Against project philosophy**: Kubernaut uses microservices, not monoliths
- ❌ **No GPU support**: cgo + CUDA is even more complex
- ❌ **Model conversion**: sentence-transformers → ONNX adds friction

**Confidence**: 20% (rejected - too much complexity for marginal benefit)

---

### **Alternative 3: Python Microservice (Flask/FastAPI)** ⭐ **RECOMMENDED**

**Approach**: Deploy a standalone Python microservice using Flask or FastAPI that exposes an HTTP API for embedding generation.

**Architecture**:
```
Data Storage Service (Go)
    ↓ HTTP POST /embed
Embedding Service (Python)
    ↓ sentence-transformers
384-dim vector
    ↓ HTTP 200 JSON
Data Storage Service (Go)
```

**Pros**:
- ✅ **Native ML ecosystem**: sentence-transformers is Python-native
- ✅ **Rapid development**: Flask/FastAPI setup in < 1 day
- ✅ **Easy debugging**: Python stack traces, no cgo complexity
- ✅ **Model flexibility**: Easy to swap models (384d → 768d → 1536d)
- ✅ **Community support**: Extensive documentation, examples, tutorials
- ✅ **Kubernetes-native**: Standard container deployment (no special build)
- ✅ **Independent scaling**: Scale embedding service separately from Data Storage
- ✅ **GPU support**: Easy to add CUDA support for faster inference
- ✅ **Future-proof**: Can add A/B testing, model versioning, batch processing
- ✅ **Air-gapped support**: Deploy in isolated environments
- ✅ **Microservices alignment**: Fits Kubernaut's architecture (10+ services already)

**Cons**:
- ⚠️ **Additional service**: +1 microservice to maintain (now 11 services)
- ⚠️ **Network latency**: HTTP call overhead (~10-50ms)
- ⚠️ **Python runtime**: Larger container image (~500MB vs ~20MB Go)
- ⚠️ **Memory footprint**: ~200MB RAM for model + runtime

**Confidence**: 90% (approved - best fit for Kubernaut)

---

## ✅ **Decision**

**APPROVED: Alternative 3** - Python Microservice (Flask/FastAPI)

**Rationale**:

1. **Microservices Architecture Alignment**:
   - Kubernaut already has 10+ microservices; one more is manageable
   - Separation of concerns: ML workload isolated from Go business logic
   - Independent scaling: Embedding service can scale based on load

2. **Development Speed**:
   - Python ML stack is mature and well-documented
   - sentence-transformers library is production-ready
   - Flask/FastAPI setup is straightforward (< 1 day)

3. **Operational Simplicity**:
   - Standard Kubernetes deployment (no cgo build issues)
   - Easy to monitor, debug, and troubleshoot
   - Container-based deployment (consistent with other services)

4. **Future-Proof**:
   - Easy to add GPU support for faster inference
   - Simple to experiment with different models
   - Can add batch processing, A/B testing, model versioning

5. **Network Latency Trade-off Acceptable**:
   - 10-50ms HTTP overhead is acceptable given 2.5s total latency budget
   - Embedding generation is ~100-200ms; network adds ~10-50ms (5-25% overhead)
   - Total: ~150-250ms for embedding (well within budget)

**Key Insight**: The marginal network latency cost (10-50ms) is vastly outweighed by the benefits of operational simplicity, development speed, and future flexibility. Kubernaut's microservices architecture is designed for this pattern.

---

## 🏗️ **Implementation**

### **Primary Implementation Files**

**New Service**:
- `cmd/embedding-service/main.py` - Service entry point
- `pkg/embedding/server.py` - Flask/FastAPI server
- `pkg/embedding/model.py` - sentence-transformers model wrapper
- `pkg/embedding/config.py` - Configuration management
- `requirements.txt` - Python dependencies
- `Dockerfile` - Container image definition
- `deploy/embedding-service.yaml` - Kubernetes deployment manifest

**Integration with Data Storage**:
- `pkg/datastorage/embedding/client.go` - HTTP client implementation
- `pkg/datastorage/embedding/interfaces.go` - Already exists (no changes)

### **Service Specification**

**Service Name**: `embedding-service`
**Port**: 8086
**Image**: `quay.io/jordigilh/embedding-service:v1.0`

**API Endpoint**:
```
POST /api/v1/embed
Content-Type: application/json

Request:
{
  "text": "Playbook content to embed",
  "model": "all-MiniLM-L6-v2"  // optional, defaults to all-MiniLM-L6-v2
}

Response (200 OK):
{
  "embedding": [0.123, -0.456, ...],  // 384 float32 values
  "dimensions": 384,
  "model": "all-MiniLM-L6-v2",
  "processing_time_ms": 120
}

Response (400 Bad Request):
{
  "error": "text field is required"
}

Response (500 Internal Server Error):
{
  "error": "model loading failed"
}
```

**Health Endpoints**:
```
GET /health/liveness  → 200 OK (service is running)
GET /health/readiness → 200 OK (model loaded, ready to serve)
GET /metrics          → Prometheus metrics
```

### **Data Flow**

1. **Data Storage receives playbook search request**
   - User: `GET /api/v1/playbooks/search?query=OOMKilled pod recovery`

2. **Data Storage calls Embedding Service**
   - HTTP POST to `http://embedding-service:8086/api/v1/embed`
   - Request body: `{"text": "OOMKilled pod recovery"}`

3. **Embedding Service processes request**
   - Load sentence-transformers model (cached in memory)
   - Tokenize input text
   - Generate 384-dimensional embedding vector
   - Return JSON response with vector

4. **Data Storage performs semantic search**
   - Use embedding vector for pgvector similarity search
   - Query: `SELECT * FROM playbook_catalog ORDER BY embedding <=> $1 LIMIT 10`
   - Return top-k matching playbooks

5. **Total Latency Breakdown**:
   - Embedding generation: ~100-200ms
   - Network (Data Storage → Embedding): ~10-50ms
   - pgvector search: ~50-100ms
   - Network (Data Storage → Client): ~10-50ms
   - **Total**: ~170-400ms (well within 2.5s budget)

### **Graceful Degradation**

**Scenario 1: Embedding Service Unavailable**
- **Behavior**: Data Storage returns 503 Service Unavailable
- **Error**: `{"error": "embedding service unavailable", "type": "service_unavailable"}`
- **Mitigation**: Retry with exponential backoff (1s, 2s, 4s)

**Scenario 2: Embedding Service Timeout**
- **Timeout**: 5 seconds
- **Behavior**: Data Storage returns 504 Gateway Timeout
- **Mitigation**: Circuit breaker pattern (open after 5 consecutive failures)

**Scenario 3: Model Loading Failure**
- **Behavior**: Embedding Service returns 503 Service Unavailable
- **Health Check**: `/health/readiness` returns 503
- **Kubernetes**: Marks pod as not ready, stops routing traffic

---

## 📊 **Consequences**

### **Positive**

- ✅ **Rapid Development**: Python ML stack enables < 1 day implementation
- ✅ **Operational Simplicity**: Standard Kubernetes deployment, no cgo issues
- ✅ **Future Flexibility**: Easy to experiment with models, add GPU, A/B testing
- ✅ **Debugging Ease**: Python stack traces, no mixed language debugging
- ✅ **Community Support**: Extensive sentence-transformers documentation
- ✅ **Independent Scaling**: Scale embedding service based on load
- ✅ **Air-Gapped Support**: Deploy in isolated environments (no external API)

### **Negative**

- ⚠️ **Additional Service**: +1 microservice to maintain (11 total)
  - **Mitigation**: Standard deployment patterns, automated monitoring
- ⚠️ **Network Latency**: ~10-50ms HTTP overhead
  - **Mitigation**: Acceptable within 2.5s budget; can optimize with connection pooling
- ⚠️ **Python Runtime**: ~500MB container image
  - **Mitigation**: Use slim Python base image, multi-stage builds
- ⚠️ **Memory Footprint**: ~200MB RAM for model
  - **Mitigation**: Acceptable for modern Kubernetes nodes; can use smaller models if needed

### **Neutral**

- 🔄 **Model Updates**: Requires container rebuild and redeployment
- 🔄 **Language Boundary**: Go ↔ Python via HTTP (standard microservices pattern)
- 🔄 **Monitoring**: Requires Prometheus metrics, logging, health checks (standard for all services)

---

## 🧪 **Validation Results**

### **Confidence Assessment Progression**

- **Initial assessment**: 85% confidence (before detailed analysis)
- **After alternatives analysis**: 90% confidence (network latency acceptable, operational benefits clear)
- **After implementation review**: 95% confidence (expected after V1.0 deployment)

### **Key Validation Points**

- ✅ **Latency Budget**: 10-50ms network overhead is 2-20% of 2.5s total budget
- ✅ **Microservices Alignment**: Fits Kubernaut's architecture (10+ services already)
- ✅ **Development Speed**: Python ML stack is mature and well-documented
- ✅ **Operational Simplicity**: Standard Kubernetes deployment patterns
- ✅ **Future-Proof**: Easy to add GPU, model versioning, batch processing

---

## 🔗 **Related Decisions**

- **Builds On**: BR-STORAGE-012 (Playbook Semantic Search)
- **Builds On**: DD-STORAGE-008 (Playbook Catalog Schema)
- **Builds On**: DD-STORAGE-012 (Critical Label Filtering - PoC validated sentence-transformers)
- **Supports**: AUDIT_TRACE_SEMANTIC_SEARCH_IMPLEMENTATION_PLAN_V1.4 (Day 4 implementation)
- **Supersedes**: None (new decision)

---

## 📋 **Review & Evolution**

### **When to Revisit**

- If **latency becomes a bottleneck** (> 500ms for embedding generation)
  - **Action**: Consider caching, batch processing, or GPU acceleration
- If **model size grows significantly** (> 1GB)
  - **Action**: Evaluate model quantization or distillation
- If **cost becomes prohibitive** (> $100/month for compute)
  - **Action**: Re-evaluate cgo approach or external API
- If **air-gapped deployment is required**
  - **Action**: Already supported; no changes needed

### **Success Metrics**

- **Latency**: p95 embedding generation < 250ms
- **Availability**: 99.9% uptime (same as Data Storage Service)
- **Throughput**: Handle 100 concurrent requests without degradation
- **Memory**: < 300MB RAM per pod
- **CPU**: < 0.5 CPU cores per pod (idle), < 2 CPU cores (peak)

---

## 📝 **Business Requirements**

### **New BRs Created**

#### **BR-EMBEDDING-001: Embedding Generation API**
- **Category**: EMBEDDING
- **Priority**: P0 (blocking for Data Storage V1.0)
- **Description**: MUST provide HTTP API for generating 384-dimensional embeddings from text
- **Acceptance Criteria**:
  - POST /api/v1/embed endpoint accepts text input
  - Returns 384-dimensional float32 vector
  - p95 latency < 250ms
  - Supports concurrent requests (100+)

#### **BR-EMBEDDING-002: sentence-transformers Model Support**
- **Category**: EMBEDDING
- **Priority**: P0 (blocking for Data Storage V1.0)
- **Description**: MUST use sentence-transformers `all-MiniLM-L6-v2` model for embedding generation
- **Acceptance Criteria**:
  - Model loaded on service startup
  - Model cached in memory for performance
  - Graceful degradation if model loading fails

#### **BR-EMBEDDING-003: Health Check Endpoints**
- **Category**: EMBEDDING
- **Priority**: P0 (blocking for Data Storage V1.0)
- **Description**: MUST provide liveness and readiness health check endpoints
- **Acceptance Criteria**:
  - GET /health/liveness returns 200 if service is running
  - GET /health/readiness returns 200 if model is loaded
  - Kubernetes uses health checks for pod lifecycle management

#### **BR-EMBEDDING-004: Prometheus Metrics**
- **Category**: EMBEDDING
- **Priority**: P1 (required for production)
- **Description**: MUST expose Prometheus metrics for observability
- **Acceptance Criteria**:
  - GET /metrics endpoint exposes metrics
  - Metrics include: request count, latency histogram, error rate
  - Metrics follow Prometheus naming conventions

#### **BR-EMBEDDING-005: Error Handling**
- **Category**: EMBEDDING
- **Priority**: P0 (blocking for Data Storage V1.0)
- **Description**: MUST handle errors gracefully and return appropriate HTTP status codes
- **Acceptance Criteria**:
  - 400 Bad Request for invalid input (missing text field)
  - 500 Internal Server Error for model failures
  - 503 Service Unavailable during startup (model loading)
  - Error responses include descriptive error messages

---

## 🚀 **Next Steps**

1. ✅ **DD-EMBEDDING-001 Approved** (this document)
2. 🚧 **Create Embedding Service Implementation Plan** (EMBEDDING_SERVICE_IMPLEMENTATION_PLAN_V1.0.md)
3. 🚧 **Implement Embedding Service** (Python microservice, 1-2 days)
4. 🚧 **Integrate with Data Storage** (HTTP client, Day 4 of Data Storage plan)
5. 🚧 **Deploy to Development Environment** (Kubernetes manifest)
6. 🚧 **Performance Testing** (validate < 250ms p95 latency)

---

**Document Version**: 1.0
**Last Updated**: November 14, 2025
**Status**: ✅ **APPROVED** (90% confidence, ready for implementation)
**Next Review**: After V1.0 deployment (performance validation)

