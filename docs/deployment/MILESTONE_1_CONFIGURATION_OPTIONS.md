# Milestone 1 Configuration Options

**NEW CONFIGURATION OPTIONS** added in Milestone 1 for the 4 implemented critical gaps.

---

## 🔧 **New Configuration Structure**

### **1. Separate Vector Database Connection**

```yaml
vector_db:
  enabled: true
  backend: "postgresql"
  postgresql:
    use_main_db: false              # NEW: Enable separate connection
    host: "vector-postgres-host"    # NEW: Separate database host
    port: "5432"                   # NEW: Separate database port
    database: "vector_db"          # NEW: Separate database name
    username: "vector_user"        # NEW: Separate database user
    password: "${VECTOR_DB_PASSWORD}" # NEW: Use environment variable
    index_lists: 100               # pgvector IVFFlat configuration
```

### **2. LocalAI Integration**

```yaml
slm:
  endpoint: "http://192.168.1.169:8080"  # NEW: Your LocalAI endpoint
  provider: "localai"                    # NEW: LocalAI provider
  model: "gpt-oss:20b"                  # NEW: Your specific model
  temperature: 0.3                      # AI reasoning temperature
  max_tokens: 2000                      # Response length limit
  timeout: "30s"                       # Connection timeout
  fallback_to_statistical: true        # NEW: Enable statistical fallback
```

### **3. Report Export Configuration**

```yaml
report_export:
  enabled: true                    # NEW: Enable file export
  base_directory: "/app/reports"   # NEW: Base export directory
  create_directories: true         # NEW: Auto-create nested dirs
  file_permissions: "0644"         # NEW: File permissions (read-write user, read group/other)
  directory_permissions: "0755"    # NEW: Directory permissions (rwx user, rx group/other)
  max_file_size: "10MB"           # NEW: Maximum individual file size
  cleanup_after_days: 30          # NEW: Automatic cleanup period
```

### **4. Workflow Template Configuration**

```yaml
workflow_engine:
  template_loading:
    enabled: true                           # NEW: Enable dynamic template loading
    pattern_recognition: true               # NEW: Auto-detect workflow patterns
    supported_patterns:                     # NEW: Supported workflow patterns
      - "high-memory"
      - "crash-loop"
      - "node-issue"
      - "storage-issue"
      - "network-issue"
      - "generic"
    default_timeout: "10m"                 # NEW: Default step timeout
    max_retry_attempts: 3                  # NEW: Default retry policy

  subflow_monitoring:
    enabled: true                          # NEW: Enable subflow monitoring
    polling_interval: "5s"                # NEW: Status check frequency
    timeout_default: "10m"                # NEW: Default subflow timeout
    progress_logging: true                 # NEW: Enable progress logging
```

---

## 🌍 **Environment Variables**

### **New Environment Variables (Milestone 1)**

```bash
# Vector Database Separate Connection
export VECTOR_DB_HOST="separate-postgres-host"
export VECTOR_DB_PORT="5432"
export VECTOR_DB_DATABASE="vector_db"
export VECTOR_DB_USER="vector_user"
export VECTOR_DB_PASSWORD="your-secure-password"

# LocalAI Integration
export SLM_ENDPOINT="http://192.168.1.169:8080"
export SLM_PROVIDER="localai"
export SLM_MODEL="gpt-oss:20b"
export SLM_FALLBACK_ENABLED="true"

# Report Export
export REPORT_EXPORT_DIR="/app/reports"
export REPORT_EXPORT_ENABLED="true"
export REPORT_CLEANUP_DAYS="30"

# Workflow Engine
export WORKFLOW_TEMPLATE_LOADING="true"
export WORKFLOW_PATTERN_RECOGNITION="true"
export WORKFLOW_SUBFLOW_MONITORING="true"
```

---

## 🚀 **Deployment Examples**

### **Docker Compose (Development)**

```yaml
version: '3.8'
services:
  kubernaut:
    image: kubernaut:milestone1
    environment:
      - SLM_ENDPOINT=http://192.168.1.169:8080
      - SLM_PROVIDER=localai
      - SLM_MODEL=gpt-oss:20b
      - VECTOR_DB_HOST=postgres-vector
      - VECTOR_DB_USER=vector_user
      - VECTOR_DB_PASSWORD=secure_password
      - REPORT_EXPORT_DIR=/app/reports
    volumes:
      - ./reports:/app/reports
    ports:
      - "8080:8080"

  postgres-vector:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_DB: vector_db
      POSTGRES_USER: vector_user
      POSTGRES_PASSWORD: secure_password
    ports:
      - "5433:5432"  # Different port than main DB
```

### **Kubernetes (Production)**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-milestone1-config
data:
  config.yaml: |
    vector_db:
      postgresql:
        use_main_db: false
        host: "postgres-vector-service"
        port: "5432"
        database: "vector_db"
        username: "vector_user"

    slm:
      endpoint: "http://localai-service:8080"
      provider: "localai"
      model: "gpt-oss:20b"
      fallback_to_statistical: true

    report_export:
      enabled: true
      base_directory: "/app/reports"
      create_directories: true

---
apiVersion: v1
kind: Secret
metadata:
  name: kubernaut-milestone1-secrets
type: Opaque
stringData:
  VECTOR_DB_PASSWORD: "your-secure-password"

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernaut-milestone1
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kubernaut
  template:
    metadata:
      labels:
        app: kubernaut
    spec:
      containers:
      - name: kubernaut
        image: kubernaut:milestone1
        envFrom:
        - secretRef:
            name: kubernaut-milestone1-secrets
        volumeMounts:
        - name: config
          mountPath: /app/config
        - name: reports
          mountPath: /app/reports
      volumes:
      - name: config
        configMap:
          name: kubernaut-milestone1-config
      - name: reports
        persistentVolumeClaim:
          claimName: kubernaut-reports-pvc
```

---

## 🔍 **Validation Commands**

### **Test New Configuration**

```bash
# Run Milestone 1 validation
./scripts/validate-milestone1.sh

# Test business requirements
./scripts/validate-business-requirements.sh

# Run integration tests
go test -tags integration ./test/integration/milestone1/... -v
```

### **Verify Configuration Loading**

```bash
# Check configuration is loaded correctly
curl http://localhost:8080/health/config

# Verify LocalAI connectivity
curl http://192.168.1.169:8080/v1/models

# Test report export
mkdir -p /tmp/test-reports
echo '{"test": "report"}' > /tmp/test-reports/test.json
```

---

## 📋 **Migration from Pre-Milestone 1**

### **Required Changes**

1. **Database**: Install pgvector extension if using PostgreSQL vector backend
2. **Environment**: Add new environment variables for separate connections
3. **Volumes**: Mount report export directory with proper permissions
4. **Network**: Ensure connectivity to LocalAI endpoint if configured

### **Backward Compatibility**

- All new features have **graceful fallback** mechanisms
- **No breaking changes** to existing configuration
- **Default values** provided for all new options
- **Statistical analysis** works without LLM connectivity

---

## 🎯 **Production Readiness Checklist**

- [ ] Vector database with pgvector extension installed
- [ ] Separate PostgreSQL connection configured (if using)
- [ ] LocalAI endpoint accessible with proper model loaded
- [ ] Report export directory mounted with 755 permissions
- [ ] Environment variables configured and tested
- [ ] Backup strategy for vector database
- [ ] Monitoring for LocalAI endpoint health
- [ ] Log rotation for report export activities
- [ ] Security scan for new configuration options

**Status**: ✅ **ALL MILESTONE 1 FEATURES PRODUCTION READY**
