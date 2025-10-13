# Data Storage Service: Deployment Prerequisites

**Service**: Data Storage Service
**Version**: V1.0
**Last Updated**: October 13, 2025
**Status**: Production-Ready

---

## 📋 Overview

The Data Storage Service requires **PostgreSQL 16.x+ with pgvector 0.5.1+** for vector similarity search using HNSW indexes. This document outlines strict version requirements and deployment prerequisites.

---

## 🚨 Critical Requirements

### **Mandatory Versions**

| Component | Required Version | Recommended | Status |
|-----------|-----------------|-------------|--------|
| **PostgreSQL** | 16.x+ | 16.2+ | ✅ **ENFORCED** |
| **pgvector** | 0.5.1+ | 0.6.0+ | ✅ **ENFORCED** |
| **Memory** | 512MB+ shared_buffers | 1GB+ | ⚠️ **VALIDATED** |

### **Unsupported Versions**

| Component | Unsupported | Why |
|-----------|------------|-----|
| **PostgreSQL** | 15.x and below | No HNSW support or incomplete implementation |
| **pgvector** | 0.5.0 and below | HNSW not available or buggy |

**Important**: The application will **fail to start** if version requirements are not met.

---

## ✅ Pre-Deployment Validation

The Data Storage Service performs automatic validation during startup:

### **Validation Checks**

1. ✅ **PostgreSQL Version**: Ensures PostgreSQL ≥ 16.0
2. ✅ **pgvector Version**: Ensures pgvector ≥ 0.5.1
3. ✅ **HNSW Index Support**: Dry-run test creates temporary HNSW index
4. ⚠️ **Memory Configuration**: Warns if shared_buffers < 1GB (non-blocking)

### **Startup Behavior**

**Valid Environment**:
```
INFO  PostgreSQL version validated  version=PostgreSQL 16.1... major=16 hnsw_supported=true
INFO  pgvector version validated  version=0.5.1 hnsw_supported=true
DEBUG HNSW index creation test passed
INFO  PostgreSQL and pgvector validation complete - HNSW support confirmed
INFO  memory configuration optimal for HNSW  shared_buffers=1GB
🚀 Data Storage Service ready
```

**Invalid Environment** (PostgreSQL < 16):
```
ERROR HNSW validation failed: PostgreSQL version 15 is not supported. Required: PostgreSQL 16.x or higher. Current: PostgreSQL 15.4 on x86_64-pc-linux-gnu. Please upgrade to PostgreSQL 16+ for HNSW vector index support
🛑 Service FAILED to start
Exit code: 1
```

**Invalid Environment** (pgvector < 0.5.1):
```
INFO  PostgreSQL version validated  version=PostgreSQL 16.1... major=16 hnsw_supported=true
ERROR HNSW validation failed: pgvector version 0.5.0 is not supported. Required: 0.5.1 or higher. Please upgrade pgvector to 0.5.1+ for HNSW support
🛑 Service FAILED to start
Exit code: 1
```

---

## 🔧 Installation Options

### **Option 1: Docker/Podman (Recommended for Development)**

```bash
# Start PostgreSQL 16 with pgvector
podman run -d \
  --name kubernaut-postgres \
  -e POSTGRES_PASSWORD=kubernaut \
  -e POSTGRES_DB=kubernaut \
  -e POSTGRES_SHARED_BUFFERS=1GB \
  -p 5432:5432 \
  pgvector/pgvector:pg16

# Verify installation
podman exec kubernaut-postgres psql -U postgres -c "SELECT version();"
# Expected: PostgreSQL 16.x...

podman exec kubernaut-postgres psql -U postgres -c "SELECT extversion FROM pg_extension WHERE extname = 'vector';"
# Expected: 0.5.1 or higher
```

---

### **Option 2: Cloud Managed PostgreSQL**

#### **AWS RDS**

1. **Create RDS Instance**:
   - Select **PostgreSQL 16.x**
   - Instance class: `db.t3.medium` or larger (for 1GB+ memory)
   - Storage: 100GB+ SSD

2. **Enable pgvector**:
   ```sql
   CREATE EXTENSION IF NOT EXISTS vector;
   SELECT extversion FROM pg_extension WHERE extname = 'vector';
   -- Verify: 0.5.1+
   ```

3. **Configure Memory**:
   - Modify parameter group: `shared_buffers = {DBInstanceClassMemory/4}` (minimum 1GB)

4. **Connection String**:
   ```
   postgres://username:password@your-rds-endpoint.amazonaws.com:5432/kubernaut?sslmode=require
   ```

#### **GCP Cloud SQL**

1. **Create Cloud SQL Instance**:
   - Select **PostgreSQL 16**
   - Machine type: `db-n1-standard-2` or larger
   - Storage: 100GB+ SSD

2. **Enable pgvector**:
   ```sql
   CREATE EXTENSION IF NOT EXISTS vector;
   SELECT extversion FROM pg_extension WHERE extname = 'vector';
   ```

3. **Configure Flags**:
   - Set `shared_buffers = 1GB` (or more)

4. **Connection String**:
   ```
   postgres://username:password@/kubernaut?host=/cloudsql/project:region:instance&sslmode=disable
   ```

#### **Azure Database for PostgreSQL**

1. **Create Flexible Server**:
   - Select **PostgreSQL 16**
   - Compute: `Standard_D2s_v3` or larger
   - Storage: 128GB+ Premium SSD

2. **Enable pgvector** (check Azure Marketplace for pgvector extension)

3. **Configure Parameters**:
   - `shared_buffers = 1GB` (or more)

4. **Connection String**:
   ```
   postgres://username:password@your-server.postgres.database.azure.com:5432/kubernaut?sslmode=require
   ```

---

### **Option 3: Self-Hosted Installation**

#### **Ubuntu/Debian**

```bash
# 1. Install PostgreSQL 16
sudo apt update
sudo apt install -y wget
sudo sh -c 'echo "deb http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list'
wget --quiet -O - https://www.postgresql.org/media/keys/ACCC4CF8.asc | sudo apt-key add -
sudo apt update
sudo apt install -y postgresql-16 postgresql-server-dev-16

# 2. Install pgvector 0.5.1+
cd /tmp
git clone --branch v0.5.1 https://github.com/pgvector/pgvector.git
cd pgvector
make
sudo make install

# 3. Configure PostgreSQL
sudo -u postgres psql -c "ALTER SYSTEM SET shared_buffers = '1GB';"
sudo systemctl restart postgresql

# 4. Enable pgvector extension
sudo -u postgres psql -d kubernaut -c "CREATE EXTENSION vector;"

# 5. Verify installation
sudo -u postgres psql -d kubernaut -c "SELECT version();"
sudo -u postgres psql -d kubernaut -c "SELECT extversion FROM pg_extension WHERE extname = 'vector';"
```

#### **Red Hat/CentOS/Fedora**

```bash
# 1. Install PostgreSQL 16
sudo dnf install -y https://download.postgresql.org/pub/repos/yum/reporpms/EL-9-x86_64/pgdg-redhat-repo-latest.noarch.rpm
sudo dnf -qy module disable postgresql
sudo dnf install -y postgresql16-server postgresql16-devel

# 2. Initialize and start PostgreSQL
sudo /usr/pgsql-16/bin/postgresql-16-setup initdb
sudo systemctl enable postgresql-16
sudo systemctl start postgresql-16

# 3. Install pgvector
cd /tmp
git clone --branch v0.5.1 https://github.com/pgvector/pgvector.git
cd pgvector
make
sudo make install

# 4. Configure PostgreSQL
sudo -u postgres psql -c "ALTER SYSTEM SET shared_buffers = '1GB';"
sudo systemctl restart postgresql-16

# 5. Enable pgvector extension
sudo -u postgres psql -d kubernaut -c "CREATE EXTENSION vector;"
```

#### **macOS (Homebrew)**

```bash
# 1. Install PostgreSQL 16
brew install postgresql@16

# 2. Start PostgreSQL
brew services start postgresql@16

# 3. Install pgvector
cd /tmp
git clone --branch v0.5.1 https://github.com/pgvector/pgvector.git
cd pgvector
make
make install

# 4. Configure PostgreSQL
psql postgres -c "ALTER SYSTEM SET shared_buffers = '1GB';"
brew services restart postgresql@16

# 5. Enable pgvector extension
createdb kubernaut
psql -d kubernaut -c "CREATE EXTENSION vector;"
```

---

## ⚙️ Configuration

### **Environment Variables**

```bash
# Database connection
POSTGRES_DSN="postgres://username:password@host:5432/kubernaut?sslmode=disable"

# Optional: Override defaults
POSTGRES_MAX_OPEN_CONNS=25
POSTGRES_MAX_IDLE_CONNS=5
POSTGRES_CONN_MAX_LIFETIME=5m
```

### **PostgreSQL Configuration** (`postgresql.conf`)

**Required Settings**:
```ini
# Memory configuration for HNSW performance
shared_buffers = 1GB              # Minimum: 512MB, Recommended: 1GB+
work_mem = 64MB                   # For index builds
maintenance_work_mem = 256MB      # For CREATE INDEX operations

# Connection settings
max_connections = 100

# Logging (optional, for troubleshooting)
log_statement = 'ddl'
log_min_duration_statement = 1000
```

**Apply Configuration**:
```bash
sudo -u postgres psql -c "ALTER SYSTEM SET shared_buffers = '1GB';"
sudo -u postgres psql -c "ALTER SYSTEM SET work_mem = '64MB';"
sudo -u postgres psql -c "ALTER SYSTEM SET maintenance_work_mem = '256MB';"
sudo systemctl restart postgresql
```

---

## 🔍 Pre-Deployment Checklist

Before deploying the Data Storage Service:

- [ ] PostgreSQL version is **16.x or higher**
- [ ] pgvector extension version is **0.5.1 or higher**
- [ ] `shared_buffers` is configured to **1GB or more**
- [ ] Database `kubernaut` exists
- [ ] pgvector extension is enabled: `CREATE EXTENSION vector;`
- [ ] Database user has permissions to create tables and indexes
- [ ] Connection string is correctly configured in environment
- [ ] Network access to PostgreSQL is allowed (firewall rules)

**Verification Commands**:
```bash
# Check PostgreSQL version
psql -U postgres -c "SELECT version();" | grep "PostgreSQL 16"

# Check pgvector version
psql -U postgres -c "SELECT extversion FROM pg_extension WHERE extname = 'vector';" | grep -E "0\.[5-9]\.[1-9]|0\.[6-9]"

# Check shared_buffers
psql -U postgres -c "SHOW shared_buffers;" | grep -E "[0-9]+GB|[5-9][0-9][0-9]MB|[0-9][0-9][0-9][0-9]MB"

# Test HNSW index creation
psql -U postgres -d kubernaut << EOF
CREATE TEMP TABLE hnsw_test (id int, embedding vector(384));
CREATE INDEX hnsw_test_idx ON hnsw_test USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);
DROP TABLE hnsw_test;
SELECT 'HNSW index test PASSED' AS result;
EOF
```

---

## 🚨 Troubleshooting

### **Issue: Service fails to start with version error**

**Error Message**:
```
ERROR: PostgreSQL version 15 is not supported. Required: PostgreSQL 16.x or higher
```

**Solution**: Upgrade to PostgreSQL 16+
- See installation instructions above for your platform
- Or use cloud provider's upgrade feature

**Migration Guide**: See [POSTGRESQL_UPGRADE_GUIDE.md](./POSTGRESQL_UPGRADE_GUIDE.md)

---

### **Issue: pgvector version too old**

**Error Message**:
```
ERROR: pgvector version 0.5.0 is not supported. Required: 0.5.1 or higher
```

**Solution**: Upgrade pgvector
```bash
# Uninstall old version
cd /tmp/pgvector
make uninstall

# Install new version
git fetch --tags
git checkout v0.5.1  # or latest
make clean
make
sudo make install

# Update extension in database
psql -U postgres -d kubernaut -c "ALTER EXTENSION vector UPDATE TO '0.5.1';"
```

---

### **Issue: Low memory warning**

**Warning Message**:
```
WARN: shared_buffers=512MB below recommended size for optimal HNSW performance
```

**Impact**: Vector search may be slower than optimal due to disk I/O.

**Solution** (Optional - not blocking):
```bash
# Edit postgresql.conf
sudo nano /etc/postgresql/16/main/postgresql.conf

# Update shared_buffers
shared_buffers = 1GB

# Restart PostgreSQL
sudo systemctl restart postgresql
```

---

## 📚 Additional Resources

- **PostgreSQL 16 Documentation**: https://www.postgresql.org/docs/16/
- **pgvector GitHub**: https://github.com/pgvector/pgvector
- **HNSW Index Documentation**: https://github.com/pgvector/pgvector#hnsw
- **Troubleshooting Guide**: [VERSION_ERRORS.md](../troubleshooting/DATASTORAGE_VERSION_ERRORS.md)
- **Performance Tuning**: [DATASTORAGE_PERFORMANCE.md](./DATASTORAGE_PERFORMANCE.md)

---

## ✅ Success Criteria

After deployment, verify:
1. ✅ Service starts without errors
2. ✅ Version validation passes
3. ✅ HNSW index creation succeeds
4. ✅ Semantic search queries return results in <50ms

**Test Command**:
```bash
# Run integration tests
make test-integration-datastorage

# Expected output:
# ✅ PostgreSQL 16 ready
# ✅ Version validation passed
# ✅ Integration tests PASSED
```

---

**Deployment Status**: ✅ Ready for production with PostgreSQL 16+ and pgvector 0.5.1+

