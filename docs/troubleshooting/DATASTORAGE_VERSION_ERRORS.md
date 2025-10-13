# Data Storage Service: Version Compatibility Errors

**Service**: Data Storage Service
**Last Updated**: October 13, 2025
**Related**: [DATASTORAGE_PREREQUISITES.md](../deployment/DATASTORAGE_PREREQUISITES.md)

---

## 📋 Overview

This guide provides troubleshooting steps for PostgreSQL and pgvector version compatibility errors in the Data Storage Service. The service requires **PostgreSQL 16.x+ with pgvector 0.5.1+** for HNSW vector index support.

---

## 🚨 Common Version Errors

### **Error 1: PostgreSQL Version Not Supported**

**Error Message**:
```
ERROR: PostgreSQL version 15 is not supported. Required: PostgreSQL 16.x or higher. Current: PostgreSQL 15.4 on x86_64-pc-linux-gnu. Please upgrade to PostgreSQL 16+ for HNSW vector index support
FATAL: Failed to initialize Data Storage Service
Exit code: 1
```

**Cause**: PostgreSQL version is below 16.0.

**Impact**: Service will not start.

---

#### **Solution 1: Upgrade PostgreSQL (Docker/Podman)**

If using containerized PostgreSQL:

```bash
# Stop old container
podman stop kubernaut-postgres
podman rm kubernaut-postgres

# Start PostgreSQL 16 container
podman run -d \
  --name kubernaut-postgres \
  -e POSTGRES_PASSWORD=kubernaut \
  -e POSTGRES_DB=kubernaut \
  -e POSTGRES_SHARED_BUFFERS=1GB \
  -p 5432:5432 \
  pgvector/pgvector:pg16

# Verify version
podman exec kubernaut-postgres psql -U postgres -c "SELECT version();"
# Expected: PostgreSQL 16.x...
```

---

#### **Solution 2: Upgrade PostgreSQL (Ubuntu/Debian)**

```bash
# 1. Backup your data
sudo -u postgres pg_dumpall > /backup/all_databases.sql

# 2. Install PostgreSQL 16
sudo sh -c 'echo "deb http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list'
wget --quiet -O - https://www.postgresql.org/media/keys/ACCC4CF8.asc | sudo apt-key add -
sudo apt update
sudo apt install -y postgresql-16 postgresql-server-dev-16

# 3. Stop old PostgreSQL
sudo systemctl stop postgresql@15-main

# 4. Upgrade cluster
sudo pg_upgradecluster 15 main

# 5. Start PostgreSQL 16
sudo systemctl start postgresql@16-main

# 6. Verify version
sudo -u postgres psql -c "SELECT version();"

# 7. Remove old version (optional, after testing)
# sudo apt remove postgresql-15
```

---

#### **Solution 3: Upgrade PostgreSQL (Cloud Providers)**

**AWS RDS**:
1. Navigate to RDS Console → Select your instance
2. Click "Modify"
3. Under "DB engine version", select **PostgreSQL 16.x**
4. Apply changes (choose immediate or maintenance window)
5. Wait for upgrade to complete (~10-30 minutes)

**GCP Cloud SQL**:
1. Navigate to Cloud SQL Console → Select your instance
2. Click "Edit"
3. Under "Database version", select **PostgreSQL 16**
4. Save changes
5. Restart instance if required

**Azure Database for PostgreSQL**:
1. Navigate to Azure Portal → Select your server
2. Click "Configuration"
3. Under "Version", select **PostgreSQL 16**
4. Apply changes
5. Server will restart automatically

---

### **Error 2: pgvector Version Not Supported**

**Error Message**:
```
ERROR: pgvector version 0.5.0 is not supported. Required: 0.5.1 or higher. Please upgrade pgvector to 0.5.1+ for HNSW support
FATAL: Failed to initialize Data Storage Service
Exit code: 1
```

**Cause**: pgvector extension version is below 0.5.1.

**Impact**: Service will not start.

---

#### **Solution 1: Upgrade pgvector (Self-Hosted)**

```bash
# 1. Download pgvector 0.5.1+
cd /tmp
git clone https://github.com/pgvector/pgvector.git
cd pgvector
git fetch --tags
git checkout v0.5.1  # or v0.6.0 for latest

# 2. Build and install
make clean
make
sudo make install

# 3. Restart PostgreSQL
sudo systemctl restart postgresql

# 4. Update extension in database
psql -U postgres -d kubernaut << EOF
ALTER EXTENSION vector UPDATE TO '0.5.1';
SELECT extversion FROM pg_extension WHERE extname = 'vector';
EOF

# Expected output: 0.5.1 or higher
```

---

#### **Solution 2: Upgrade pgvector (Docker/Podman)**

```bash
# Stop and remove old container
podman stop kubernaut-postgres
podman rm kubernaut-postgres

# Use latest pgvector image (includes pgvector 0.5.1+)
podman run -d \
  --name kubernaut-postgres \
  -e POSTGRES_PASSWORD=kubernaut \
  -e POSTGRES_DB=kubernaut \
  -e POSTGRES_SHARED_BUFFERS=1GB \
  -p 5432:5432 \
  pgvector/pgvector:pg16

# Verify pgvector version
podman exec kubernaut-postgres psql -U postgres -c "SELECT extversion FROM pg_extension WHERE extname = 'vector';"
```

---

#### **Solution 3: Upgrade pgvector (Cloud Providers)**

**AWS RDS**: pgvector is available through shared extensions
```sql
-- Drop old extension (backup data first!)
DROP EXTENSION vector;

-- Create new extension
CREATE EXTENSION vector VERSION '0.5.1';

-- Verify
SELECT extversion FROM pg_extension WHERE extname = 'vector';
```

**GCP Cloud SQL / Azure**: Check cloud provider marketplace for pgvector 0.5.1+ availability

---

### **Error 3: pgvector Extension Not Installed**

**Error Message**:
```
ERROR: pgvector extension not installed: pgvector extension is not installed. Install with: CREATE EXTENSION vector
FATAL: Failed to initialize Data Storage Service
Exit code: 1
```

**Cause**: pgvector extension is not enabled in the database.

**Impact**: Service will not start.

---

#### **Solution: Enable pgvector Extension**

```bash
# Connect to database
psql -U postgres -d kubernaut

# Enable pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

# Verify installation
SELECT extversion FROM pg_extension WHERE extname = 'vector';
-- Expected: 0.5.1 or higher

# Exit
\q
```

If `CREATE EXTENSION` fails:
```
ERROR:  could not open extension control file "/usr/share/postgresql/16/extension/vector.control": No such file or directory
```

Then pgvector is not installed. Install it first:
```bash
# Ubuntu/Debian
cd /tmp
git clone --branch v0.5.1 https://github.com/pgvector/pgvector.git
cd pgvector
make
sudo make install
sudo systemctl restart postgresql

# Then try CREATE EXTENSION again
psql -U postgres -d kubernaut -c "CREATE EXTENSION vector;"
```

---

### **Error 4: HNSW Index Creation Test Failed**

**Error Message**:
```
ERROR: HNSW index creation test failed: HNSW index creation failed: ERROR: access method "hnsw" does not exist. Your PostgreSQL/pgvector installation does not support HNSW
FATAL: Failed to initialize Data Storage Service
Exit code: 1
```

**Cause**: HNSW index type is not available (usually due to old pgvector version or PostgreSQL version).

**Impact**: Service will not start.

---

#### **Solution: Verify and Upgrade**

```bash
# 1. Check PostgreSQL version
psql -U postgres -c "SELECT version();"
# Must be: PostgreSQL 16.x or higher

# 2. Check pgvector version
psql -U postgres -c "SELECT extversion FROM pg_extension WHERE extname = 'vector';"
# Must be: 0.5.1 or higher

# 3. Test HNSW manually
psql -U postgres -d kubernaut << EOF
CREATE TEMP TABLE hnsw_test (id int, embedding vector(384));
CREATE INDEX hnsw_test_idx ON hnsw_test USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);
\d hnsw_test
DROP TABLE hnsw_test;
EOF

# If this fails, upgrade PostgreSQL and/or pgvector
```

---

### **Warning: Low Memory Configuration**

**Warning Message**:
```
WARN: shared_buffers=512MB below recommended size for optimal HNSW performance
      current=512MB recommended=1GB+ 
      impact=vector search may be slower than optimal due to disk I/O
      action=consider increasing shared_buffers in postgresql.conf
```

**Cause**: PostgreSQL `shared_buffers` is less than 1GB.

**Impact**: Vector search may be slower than optimal (~100ms vs ~30ms).

**Severity**: ⚠️ **Non-blocking** (service will still start)

---

#### **Solution: Increase shared_buffers (Optional)**

```bash
# Option 1: Using ALTER SYSTEM (recommended)
sudo -u postgres psql << EOF
ALTER SYSTEM SET shared_buffers = '1GB';
EOF
sudo systemctl restart postgresql

# Option 2: Edit postgresql.conf manually
sudo nano /etc/postgresql/16/main/postgresql.conf

# Find and update:
shared_buffers = 1GB

# Save and restart
sudo systemctl restart postgresql

# Verify
psql -U postgres -c "SHOW shared_buffers;"
# Expected: 1GB
```

**Note**: Increasing `shared_buffers` may require adjusting system `shmmax`:
```bash
# Check current limit
cat /proc/sys/kernel/shmmax

# If too low, increase (requires root)
sudo sysctl -w kernel.shmmax=1073741824  # 1GB in bytes
sudo sysctl -w kernel.shmall=262144       # pages

# Make permanent
echo "kernel.shmmax = 1073741824" | sudo tee -a /etc/sysctl.conf
echo "kernel.shmall = 262144" | sudo tee -a /etc/sysctl.conf
```

---

## 🔍 Diagnostic Commands

### **Quick Health Check**

```bash
#!/bin/bash
# datastorage-health-check.sh

echo "=== Data Storage Service Health Check ==="
echo ""

# Check PostgreSQL version
echo "PostgreSQL Version:"
psql -U postgres -t -c "SELECT version();" 2>&1 | head -1
PG_MAJOR=$(psql -U postgres -t -c "SELECT version();" 2>&1 | grep -oP 'PostgreSQL \K\d+' | head -1)
if [ "$PG_MAJOR" -ge 16 ]; then
    echo "✅ PostgreSQL $PG_MAJOR (supported)"
else
    echo "❌ PostgreSQL $PG_MAJOR (NOT supported, need 16+)"
fi
echo ""

# Check pgvector version
echo "pgvector Version:"
PGVECTOR_VER=$(psql -U postgres -t -c "SELECT extversion FROM pg_extension WHERE extname = 'vector';" 2>&1 | xargs)
if [ -n "$PGVECTOR_VER" ]; then
    echo "pgvector $PGVECTOR_VER"
    if [[ "$PGVECTOR_VER" > "0.5.0" ]]; then
        echo "✅ pgvector $PGVECTOR_VER (supported)"
    else
        echo "❌ pgvector $PGVECTOR_VER (NOT supported, need 0.5.1+)"
    fi
else
    echo "❌ pgvector not installed"
fi
echo ""

# Check shared_buffers
echo "Memory Configuration:"
SHARED_BUFFERS=$(psql -U postgres -t -c "SHOW shared_buffers;" 2>&1 | xargs)
echo "shared_buffers: $SHARED_BUFFERS"
if [[ "$SHARED_BUFFERS" =~ GB ]]; then
    echo "✅ Memory configuration optimal"
elif [[ "$SHARED_BUFFERS" =~ MB ]]; then
    MB_VALUE=$(echo $SHARED_BUFFERS | grep -oP '\d+')
    if [ "$MB_VALUE" -ge 1024 ]; then
        echo "✅ Memory configuration optimal"
    else
        echo "⚠️  Memory below recommended (1GB+)"
    fi
fi
echo ""

# Test HNSW index creation
echo "HNSW Index Support:"
HNSW_TEST=$(psql -U postgres -d kubernaut -t << EOF
CREATE TEMP TABLE hnsw_test (id int, embedding vector(384));
CREATE INDEX hnsw_test_idx ON hnsw_test USING hnsw (embedding vector_cosine_ops);
SELECT 'SUCCESS';
EOF
)
if echo "$HNSW_TEST" | grep -q "SUCCESS"; then
    echo "✅ HNSW index creation successful"
else
    echo "❌ HNSW index creation failed"
fi
echo ""

echo "=== Health Check Complete ==="
```

Make executable and run:
```bash
chmod +x datastorage-health-check.sh
./datastorage-health-check.sh
```

---

## 📚 Additional Resources

- **Prerequisites**: [DATASTORAGE_PREREQUISITES.md](../deployment/DATASTORAGE_PREREQUISITES.md)
- **PostgreSQL 16 Upgrade Guide**: https://www.postgresql.org/docs/16/upgrading.html
- **pgvector Documentation**: https://github.com/pgvector/pgvector
- **HNSW Index Tuning**: https://github.com/pgvector/pgvector#hnsw

---

## ✅ Verification After Fixes

After applying any fixes, verify the service starts successfully:

```bash
# 1. Check versions
psql -U postgres -c "SELECT version();" | grep "PostgreSQL 16"
psql -U postgres -c "SELECT extversion FROM pg_extension WHERE extname = 'vector';" | grep -E "0\.[5-9]\.[1-9]|0\.[6-9]"

# 2. Test HNSW index
psql -U postgres -d kubernaut << EOF
CREATE TEMP TABLE test (embedding vector(384));
CREATE INDEX test_idx ON test USING hnsw (embedding vector_cosine_ops);
EOF

# 3. Run integration tests
make test-integration-datastorage

# Expected output:
# ✅ PostgreSQL 16 ready
# ✅ Version validation passed
# ✅ Integration tests PASSED
```

---

**Support**: If issues persist after following this guide, check application logs for detailed error messages or contact the Kubernaut support team.

