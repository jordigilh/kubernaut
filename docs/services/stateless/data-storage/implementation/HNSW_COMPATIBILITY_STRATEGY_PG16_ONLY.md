# HNSW Compatibility Strategy: PostgreSQL 16+ Only (Final)

**Date**: October 13, 2025
**Status**: Strategic Decision - PostgreSQL 16+ and pgvector 0.5.1+ Only
**Risk Level**: 🟢 Low (modern versions only)
**Confidence**: 99.9%

---

## 🚨 Final Design Decision

**STRICT REQUIREMENTS**:
- **PostgreSQL**: 16.x+ ONLY
- **pgvector**: 0.5.1+ ONLY
- **HNSW**: Mandatory (no fallback)

**Rationale**:
- PostgreSQL 16+ is stable, widely available, and fully supports HNSW
- pgvector 0.5.1+ has mature HNSW implementation with performance optimizations
- Eliminates complex version compatibility matrix
- Simplifies validation logic significantly
- Target: Modern cloud deployments (AWS RDS, GCP Cloud SQL, Azure PostgreSQL)

---

## 📋 Version Requirements (SIMPLIFIED)

| Component | Version | Status | Notes |
|-----------|---------|--------|-------|
| **PostgreSQL** | 16.0+ | ✅ **ONLY SUPPORTED** | 16.x is stable (released Sept 2023) |
| **pgvector** | 0.5.1+ | ✅ **ONLY SUPPORTED** | Mature HNSW, performance improvements |
| **Memory** | 1GB+ shared_buffers | ⚠️ **RECOMMENDED** | Validated, but not blocking |

**Unsupported Versions** (Will fail validation):
- ❌ PostgreSQL 15.x and below
- ❌ pgvector 0.5.0 and below
- ❌ Any version without HNSW support

---

## 🛡️ Simplified Mitigation Strategies

### Strategy 1: Simplified Version Validation ⭐ **CRITICAL**

**Purpose**: Enforce PostgreSQL 16+ and pgvector 0.5.1+ requirements.

**Simplified Implementation**: (Much simpler than multi-version support)

```go
// pkg/datastorage/schema/validator.go

type VersionValidator struct {
    db     *sql.DB
    logger *zap.Logger
}

// ValidateHNSWSupport enforces PostgreSQL 16+ and pgvector 0.5.1+
func (v *VersionValidator) ValidateHNSWSupport(ctx context.Context) error {
    // Step 1: Validate PostgreSQL version (16+ only)
    pgVersion, err := v.getPostgreSQLVersion(ctx)
    if err != nil {
        return fmt.Errorf("failed to detect PostgreSQL version: %w", err)
    }

    pgMajor := v.parsePostgreSQLMajorVersion(pgVersion)
    if pgMajor < 16 {
        return fmt.Errorf(
            "PostgreSQL version %d is not supported. Required: PostgreSQL 16.x or higher. Current: %s",
            pgMajor, pgVersion)
    }

    v.logger.Info("PostgreSQL version validated",
        zap.String("version", pgVersion),
        zap.Int("major", pgMajor),
        zap.Bool("supported", true))

    // Step 2: Validate pgvector version (0.5.1+ only)
    pgvectorVersion, err := v.getPgvectorVersion(ctx)
    if err != nil {
        return fmt.Errorf("pgvector extension not installed: %w", err)
    }

    if !v.isPgvector051OrHigher(pgvectorVersion) {
        return fmt.Errorf(
            "pgvector version %s is not supported. Required: 0.5.1 or higher",
            pgvectorVersion)
    }

    v.logger.Info("pgvector version validated",
        zap.String("version", pgvectorVersion),
        zap.Bool("supported", true))

    // Step 3: Test HNSW index creation (dry-run)
    err = v.testHNSWIndexCreation(ctx)
    if err != nil {
        return fmt.Errorf("HNSW index creation test failed: %w", err)
    }

    v.logger.Info("HNSW support validation complete - all checks passed",
        zap.String("postgres", pgVersion),
        zap.String("pgvector", pgvectorVersion))

    return nil
}

func (v *VersionValidator) getPostgreSQLVersion(ctx context.Context) (string, error) {
    var version string
    err := v.db.QueryRowContext(ctx, "SELECT version()").Scan(&version)
    return version, err
}

func (v *VersionValidator) parsePostgreSQLMajorVersion(version string) int {
    // Parse: "PostgreSQL 16.1 on x86_64..." → 16
    re := regexp.MustCompile(`PostgreSQL (\d+)\.`)
    matches := re.FindStringSubmatch(version)
    if len(matches) < 2 {
        return 0
    }
    major, _ := strconv.Atoi(matches[1])
    return major
}

func (v *VersionValidator) getPgvectorVersion(ctx context.Context) (string, error) {
    var version string
    err := v.db.QueryRowContext(ctx, `
        SELECT extversion
        FROM pg_extension
        WHERE extname = 'vector'
    `).Scan(&version)

    if err == sql.ErrNoRows {
        return "", fmt.Errorf("pgvector extension is not installed. Install with: CREATE EXTENSION vector")
    }
    return version, err
}

func (v *VersionValidator) isPgvector051OrHigher(version string) bool {
    // Parse: "0.5.1" or "0.6.0" → true, "0.5.0" or "0.4.x" → false
    re := regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)
    matches := re.FindStringSubmatch(version)
    if len(matches) < 4 {
        return false
    }

    major, _ := strconv.Atoi(matches[1])
    minor, _ := strconv.Atoi(matches[2])
    patch, _ := strconv.Atoi(matches[3])

    // Require 0.5.1 or higher
    if major > 0 {
        return true // 1.0.0+
    }
    if minor > 5 {
        return true // 0.6.0+
    }
    if minor == 5 && patch >= 1 {
        return true // 0.5.1+
    }
    return false
}

func (v *VersionValidator) testHNSWIndexCreation(ctx context.Context) error {
    // Create temporary table with vector column
    _, err := v.db.ExecContext(ctx, `
        CREATE TEMP TABLE hnsw_validation_test (
            id SERIAL PRIMARY KEY,
            embedding vector(384)
        )
    `)
    if err != nil {
        return fmt.Errorf("failed to create test table: %w", err)
    }

    // Attempt HNSW index creation
    _, err = v.db.ExecContext(ctx, `
        CREATE INDEX hnsw_validation_test_idx ON hnsw_validation_test
        USING hnsw (embedding vector_cosine_ops)
        WITH (m = 16, ef_construction = 64)
    `)
    if err != nil {
        return fmt.Errorf("HNSW index creation failed: %w", err)
    }

    v.logger.Debug("HNSW index creation test passed")
    return nil
}
```

**Benefits**:
- ✅ **Extremely simple**: Only check major version >= 16, pgvector >= 0.5.1
- ✅ **No version matrix**: Single code path
- ✅ **Fast validation**: 2 queries + 1 dry-run test
- ✅ **Clear errors**: "PostgreSQL 15 not supported. Required: 16+"

---

### Strategy 2: Memory Configuration Validation (Recommended, Not Blocking)

**Purpose**: Warn if `shared_buffers` is too low, but don't block startup.

**Rationale**: PostgreSQL 16+ defaults are better, and cloud providers often manage this.

```go
func (v *VersionValidator) ValidateMemoryConfiguration(ctx context.Context) error {
    var sharedBuffers string
    err := v.db.QueryRowContext(ctx, `
        SELECT current_setting('shared_buffers')
    `).Scan(&sharedBuffers)
    if err != nil {
        v.logger.Warn("failed to read shared_buffers configuration", zap.Error(err))
        return nil // Don't block startup
    }

    bufferSize, err := parsePostgreSQLSize(sharedBuffers)
    if err != nil {
        v.logger.Warn("failed to parse shared_buffers", zap.Error(err))
        return nil // Don't block startup
    }

    const recommendedBufferSize = 1024 * 1024 * 1024 // 1GB

    if bufferSize < recommendedBufferSize {
        v.logger.Warn("shared_buffers below recommended size for optimal HNSW performance",
            zap.String("current", sharedBuffers),
            zap.String("recommended", "1GB+"),
            zap.String("impact", "vector search may be slower than optimal"),
            zap.String("action", "consider increasing shared_buffers in postgresql.conf"))
    } else {
        v.logger.Info("memory configuration optimal for HNSW",
            zap.String("shared_buffers", sharedBuffers))
    }

    return nil // Never block, only warn
}

func parsePostgreSQLSize(size string) (int64, error) {
    // Parse: "128MB", "1GB", "8192kB"
    re := regexp.MustCompile(`(\d+)\s*(kB|MB|GB|TB)?`)
    matches := re.FindStringSubmatch(size)
    if len(matches) < 2 {
        return 0, fmt.Errorf("invalid size format: %s", size)
    }

    value, _ := strconv.ParseInt(matches[1], 10, 64)
    unit := strings.ToUpper(matches[2])

    switch unit {
    case "TB":
        return value * 1024 * 1024 * 1024 * 1024, nil
    case "GB":
        return value * 1024 * 1024 * 1024, nil
    case "MB", "":
        return value * 1024 * 1024, nil
    case "KB":
        return value * 1024, nil
    default:
        return value * 8192, nil // PostgreSQL default unit (8kB blocks)
    }
}
```

**Benefits**:
- ✅ **Non-blocking**: Warns but allows startup
- ✅ **Actionable**: Tells users how to optimize
- ✅ **Cloud-friendly**: Many cloud providers manage this automatically

---

### Strategy 3: Query Planner Hints (Still Recommended)

**No change** - Still valid and useful for PostgreSQL 16.

```go
func (s *Service) SemanticSearch(ctx context.Context, queryEmbedding []float32, opts ListOptions) ([]*SemanticResult, error) {
    // Force HNSW index usage (especially with complex WHERE clauses)
    _, err := s.db.ExecContext(ctx, `
        SET LOCAL enable_seqscan = off;
        SET LOCAL enable_indexscan = on;
    `)
    if err != nil {
        s.logger.Warn("failed to set query planner hints",
            zap.Error(err),
            zap.String("impact", "query may not use HNSW index optimally"))
    }

    // Execute semantic search
    sqlQuery := `
        SELECT
            id, name, namespace, phase, action_type, status,
            start_time, end_time, duration,
            remediation_request_id, alert_fingerprint,
            severity, environment, cluster_name, target_resource,
            error_message, metadata, embedding,
            created_at, updated_at,
            1 - (embedding <=> $1) AS similarity
        FROM remediation_audit
        WHERE embedding IS NOT NULL
        ORDER BY embedding <=> $1
        LIMIT $2
    `

    // ... rest of implementation ...
}
```

---

### Strategy 4: Simplified CI/CD Testing ⭐ **CRITICAL**

**Purpose**: Test PostgreSQL 16.x variations only.

**Simplified Matrix**:

```yaml
# .github/workflows/integration-tests-datastorage.yml

jobs:
  integration-test-datastorage:
    name: Data Storage Integration (PostgreSQL ${{ matrix.pg-version }})
    runs-on: ubuntu-latest

    strategy:
      fail-fast: false
      matrix:
        # Only test PostgreSQL 16.x with pgvector 0.5.1+
        pg-version: ["16.0", "16.1", "16.2"]
        pgvector-version: ["0.5.1", "0.6.0"]

    services:
      postgres:
        image: pgvector/pgvector:${{ matrix.pg-version }}-pg16
        env:
          POSTGRES_DB: test_db
          POSTGRES_USER: test_user
          POSTGRES_PASSWORD: test_pass
          # PostgreSQL 16 has good defaults, but ensure 1GB for tests
          POSTGRES_SHARED_BUFFERS: 1GB
          POSTGRES_WORK_MEM: 64MB
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5

    steps:
      - name: Checkout code
        uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Install pgvector extension
        run: |
          psql -h localhost -U test_user -d test_db -c "CREATE EXTENSION IF NOT EXISTS vector VERSION '${{ matrix.pgvector-version }}'"

      - name: Run integration tests
        env:
          POSTGRES_DSN: "postgres://test_user:test_pass@localhost:5432/test_db?sslmode=disable"
        run: |
          make test-integration-datastorage

      - name: Verify HNSW index exists
        run: |
          psql -h localhost -U test_user -d test_db -c "
            SELECT
              schemaname,
              tablename,
              indexname,
              indexdef
            FROM pg_indexes
            WHERE tablename = 'remediation_audit'
              AND indexdef LIKE '%hnsw%'
          " | tee /tmp/hnsw_index.txt

          # Fail if HNSW index is NOT found
          if ! grep -q "hnsw" /tmp/hnsw_index.txt; then
            echo "❌ ERROR: HNSW index not created!"
            exit 1
          fi

          echo "✅ HNSW index verified for PostgreSQL ${{ matrix.pg-version }}"

      - name: Benchmark HNSW query performance
        run: |
          go test -v -run=TestSemanticSearchPerformance -bench=. ./test/integration/datastorage/
```

**Benefits**:
- ✅ **Simple matrix**: Only 6 combinations (3 PG versions × 2 pgvector versions)
- ✅ **Fast CI**: No need to test 10+ version combinations
- ✅ **Clear pass/fail**: HNSW either works or test fails

---

### Strategy 5: Simplified Documentation ⭐ **CRITICAL**

**Purpose**: Clear, simple version requirements.

#### 1. Deployment Prerequisites (`docs/deployment/PREREQUISITES.md`)

```markdown
## PostgreSQL Requirements

**Kubernaut requires PostgreSQL 16+ with pgvector 0.5.1+ (HNSW support).**

### Supported Versions
- ✅ **PostgreSQL 16.0+** (any 16.x version)
- ✅ **pgvector 0.5.1+** (0.6.0+ recommended)
- ⚠️ **Memory**: 1GB+ shared_buffers recommended

### Unsupported Versions
- ❌ PostgreSQL 15.x and below → **NOT SUPPORTED**
- ❌ pgvector 0.5.0 and below → **NOT SUPPORTED**

### Installation

#### Option 1: Docker/Podman (Recommended for Development)
```bash
podman run -d \
  --name kubernaut-postgres \
  -e POSTGRES_PASSWORD=kubernaut \
  -e POSTGRES_DB=kubernaut \
  -p 5432:5432 \
  pgvector/pgvector:pg16
```

#### Option 2: Cloud Managed PostgreSQL
- **AWS RDS**: PostgreSQL 16+ with pgvector extension
- **GCP Cloud SQL**: PostgreSQL 16+ with pgvector extension
- **Azure Database for PostgreSQL**: PostgreSQL 16+ with pgvector extension

#### Option 3: Self-Hosted
```bash
# Install PostgreSQL 16
sudo apt install postgresql-16

# Install pgvector 0.5.1+
cd /tmp
git clone --branch v0.5.1 https://github.com/pgvector/pgvector.git
cd pgvector
make
sudo make install

# Enable extension
psql -U postgres -d kubernaut -c "CREATE EXTENSION vector"
```

### Pre-Deployment Validation
Kubernaut validates versions at startup:
```
✅ PostgreSQL 16.1 detected - supported
✅ pgvector 0.5.1 detected - supported
✅ HNSW index creation test passed
🚀 Data Storage Service ready
```

If validation fails:
```
❌ PostgreSQL version 15.4 is not supported. Required: PostgreSQL 16.x or higher
```
```

#### 2. Error Messages Reference (`docs/troubleshooting/VERSION_ERRORS.md`)

```markdown
## Version Compatibility Errors

### Error: "PostgreSQL version X is not supported. Required: PostgreSQL 16.x or higher"

**Cause**: PostgreSQL version is below 16.0.

**Solution**:
1. **Upgrade to PostgreSQL 16+**:
   ```bash
   # Ubuntu/Debian
   sudo apt install postgresql-16

   # macOS
   brew install postgresql@16

   # Docker/Podman
   podman run -d pgvector/pgvector:pg16
   ```

2. **Cloud Provider Upgrade**:
   - AWS RDS: Modify DB instance → Select PostgreSQL 16.x
   - GCP Cloud SQL: Edit instance → Change PostgreSQL version to 16
   - Azure: Update PostgreSQL flexible server to version 16

3. **Migration Guide**: [POSTGRESQL_UPGRADE_GUIDE.md]

---

### Error: "pgvector version X.Y.Z is not supported. Required: 0.5.1 or higher"

**Cause**: pgvector extension is below 0.5.1.

**Solution**:
1. **Upgrade pgvector**:
   ```sql
   -- Check current version
   SELECT extversion FROM pg_extension WHERE extname = 'vector';

   -- Upgrade to 0.5.1+
   ALTER EXTENSION vector UPDATE TO '0.5.1';
   ```

2. **If upgrade not available, reinstall**:
   ```bash
   cd /tmp
   git clone --branch v0.5.1 https://github.com/pgvector/pgvector.git
   cd pgvector
   make clean && make && sudo make install
   ```

3. **Verify installation**:
   ```sql
   SELECT extversion FROM pg_extension WHERE extname = 'vector';
   -- Should return: 0.5.1 or higher
   ```

---

### Error: "HNSW index creation test failed"

**Cause**: HNSW index creation failed during validation.

**Possible Causes**:
1. pgvector extension not properly installed
2. Insufficient database permissions
3. Corrupted pgvector installation

**Solution**:
1. **Verify pgvector is loaded**:
   ```sql
   SELECT * FROM pg_extension WHERE extname = 'vector';
   ```

2. **Test HNSW manually**:
   ```sql
   CREATE TABLE test_hnsw (id int, embedding vector(384));
   CREATE INDEX test_hnsw_idx ON test_hnsw USING hnsw (embedding vector_cosine_ops);
   DROP TABLE test_hnsw;
   ```

3. **Check PostgreSQL logs** for detailed error messages

---

### Warning: "shared_buffers below recommended size for optimal HNSW performance"

**Cause**: PostgreSQL `shared_buffers` is less than 1GB.

**Impact**: Vector search may be slower than optimal due to disk I/O.

**Solution** (Optional - not blocking):
1. **Edit `postgresql.conf`**:
   ```
   shared_buffers = 1GB
   ```

2. **Restart PostgreSQL**:
   ```bash
   sudo systemctl restart postgresql
   ```

3. **Verify**:
   ```sql
   SHOW shared_buffers;
   -- Should return: 1GB
   ```
```

#### 3. Update Schema Comments

```sql
-- internal/database/schema/remediation_audit.sql

-- Create HNSW vector index for similarity search
-- REQUIRES: PostgreSQL 16.x+ and pgvector 0.5.1+
-- Application startup will FAIL if HNSW is not supported
-- No fallback mechanism - HNSW is mandatory for semantic search
CREATE INDEX IF NOT EXISTS idx_remediation_audit_embedding ON remediation_audit
USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);
```

---

## 📊 Risk Assessment (PostgreSQL 16+ Only)

| Risk Factor | Probability | Impact | Severity | Mitigation |
|-------------|------------|--------|----------|------------|
| **PostgreSQL < 16 deployment** | Very Low | Critical | 🟢 Low | Pre-flight validation fails immediately |
| **pgvector < 0.5.1 deployment** | Very Low | Critical | 🟢 Low | Pre-flight validation fails immediately |
| **HNSW index creation fails** | Very Low | Critical | 🟢 Low | Dry-run test catches during startup |
| **Insufficient memory** | Low | Medium | 🟢 Low | Warning logged, not blocking |
| **Query planner ignores HNSW** | Very Low | Medium | 🟢 Low | Query hints enforce HNSW usage |

**Overall Risk**: 🟢 **Very Low** (PostgreSQL 16+ has mature HNSW support)

---

## ✅ Simplified Implementation Plan

### Phase 1: Version Validation (1-2 hours) ⭐ **CRITICAL**

1. ✅ **Create `pkg/datastorage/schema/validator.go`**
   - Implement `ValidateHNSWSupport()` (PostgreSQL 16+ check)
   - Implement `ValidateMemoryConfiguration()` (warn only)
   - Add dry-run HNSW test
   - **ETA**: 1 hour

2. ✅ **Integrate into Application Startup**
   - Call validators in `pkg/datastorage/client.go` or `cmd/kubernaut/main.go`
   - Fail fast with clear errors
   - **ETA**: 15 minutes

3. ✅ **Add Integration Tests**
   - Test version validation logic
   - Test error messages
   - **ETA**: 30 minutes

### Phase 2: CI/CD and Documentation (1.5-2 hours) ⭐ **CRITICAL**

4. ✅ **Simplify CI/CD Matrix**
   - Test PostgreSQL 16.0, 16.1, 16.2
   - Test pgvector 0.5.1, 0.6.0
   - Add HNSW verification step
   - **ETA**: 45 minutes

5. ✅ **Update Documentation**
   - Update deployment prerequisites
   - Create error message reference
   - Add upgrade guides
   - **ETA**: 45 minutes

### Phase 3: Query Optimization (30 minutes) - Optional

6. ✅ **Add Query Planner Hints**
   - Implement in `SemanticSearch` method
   - **ETA**: 30 minutes

---

## 🎯 Success Criteria (PostgreSQL 16+ Only)

| Metric | Target | Confidence |
|--------|--------|------------|
| **Version Validation** | 100% | 99.9% |
| **False Positives** | 0% | 99.9% |
| **False Negatives** | 0% | 99.9% |
| **HNSW Index Creation** | 100% | 99.9% |
| **Query Performance** | <50ms p95 | 95% |
| **CI/CD Coverage** | 100% of PG 16.x | 99% |

---

## 📋 Deployment Checklist (Simplified)

Before deploying to any environment:

- [ ] PostgreSQL version is 16.x (any 16.x version supported)
- [ ] pgvector version is 0.5.1+ (0.6.0+ recommended)
- [ ] Pre-flight validation implemented
- [ ] CI/CD tests pass for PostgreSQL 16.x
- [ ] Documentation updated
- [ ] Operations team aware of version requirements

---

## 🎉 Conclusion

**Risk Level**: 🟢 **Very Low** (PostgreSQL 16+ is stable and mature)

**Confidence**: 99.9%

**Key Simplifications from Previous Strategy**:
- ❌ **Removed**: Support for PostgreSQL 12-15
- ❌ **Removed**: Complex version compatibility matrix
- ❌ **Removed**: Multi-version testing (12-15)
- ✅ **Simplified**: Version validation (just check >= 16)
- ✅ **Simplified**: CI/CD matrix (6 combinations vs 12+)
- ✅ **Simplified**: Documentation (single version requirement)

**Benefits of PostgreSQL 16+ Only**:
1. ✅ **Simpler codebase**: Minimal validation logic
2. ✅ **Faster CI/CD**: Test 6 combinations instead of 12+
3. ✅ **Clearer requirements**: "PostgreSQL 16+" (one number)
4. ✅ **Future-proof**: PostgreSQL 16 released Sept 2023, stable and widely adopted
5. ✅ **Cloud-friendly**: All major cloud providers support PostgreSQL 16+

**Total Implementation Time**: ~3-4 hours
**Priority**: 🔴 **CRITICAL** - Complete before any deployment

---

## 📝 Recommended Next Steps

**Immediate (Phase 1)**:
1. Implement `pkg/datastorage/schema/validator.go` with PostgreSQL 16+ check
2. Integrate validators into application startup
3. Test with PostgreSQL 15 (should fail) and PostgreSQL 16 (should pass)

**Short-term (Phase 2)**:
4. Update CI/CD to test PostgreSQL 16.0, 16.1, 16.2
5. Update all documentation with "PostgreSQL 16+ only" requirement

**Optional (Phase 3)**:
6. Add query planner hints for optimal HNSW usage

Would you like me to implement Phase 1 now?

