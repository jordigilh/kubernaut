# DD-CACHE-001: Shared Redis Library

**Date**: November 23, 2025
**Status**: ✅ **APPROVED**
**Decision Maker**: Kubernaut Architecture Team
**Authority**: DD-INFRASTRUCTURE-001 (Redis Separation), DD-INFRASTRUCTURE-002 (Data Storage Redis Strategy)
**Affects**: Gateway, Data Storage, Kubernaut Agent (KA), Signal Processing (future)
**Version**: 1.0

---

## 📋 **Status**

**✅ APPROVED** (2025-11-23)
**Last Reviewed**: 2025-11-23
**Confidence**: 95%

---

## 🎯 **Context & Problem**

### **Problem Statement**

Multiple Kubernaut services need Redis for caching, but each service is implementing Redis connection management independently. Gateway has proven patterns for:
- Connection management with lazy loading
- Graceful degradation on Redis failures
- Configuration structure
- Double-checked locking for concurrent access

**Current State**:
- ✅ **Gateway**: Fully implemented Redis patterns in `pkg/gateway/processing/deduplication.go`
- ⏸️ **Data Storage**: About to implement Redis cache for embeddings (copy-paste approach)
- 📋 **Kubernaut Agent**: Will need Redis for investigation results cache (V1.1)
- 📋 **Signal Processing**: Will need Redis for pattern detection cache (V1.1)

**Problem**: Should we extract Gateway's Redis patterns into a shared library, or continue copy-pasting?

---

## 🔍 **Alternatives Considered**

### **Alternative A: Shared Redis Library** ✅ **APPROVED**

**Approach**: Extract Gateway's Redis patterns into `pkg/cache/redis` package

**Architecture**:
```
pkg/cache/redis/
├── client.go          # Connection management, graceful degradation
├── config.go          # RedisOptions struct
├── cache.go           # Generic Cache[T] interface
├── health.go          # Health check utilities
└── client_test.go     # Shared test utilities
```

**Usage Example**:
```go
// Data Storage embedding cache
import rediscache "github.com/jordigilh/kubernaut/pkg/cache/redis"

// Create Redis client
client := rediscache.NewClient(opts, logger)

// Create typed cache
embeddingCache := rediscache.NewCache[[]float32](client, "embedding:", 24*time.Hour)

// Use cache
if cached, err := embeddingCache.Get(ctx, text); err == nil {
    return *cached, nil
}
```

**Pros**:
- ✅ **DRY Principle**: Single source of truth for Redis patterns
- ✅ **Consistency**: All services use same connection management
- ✅ **Maintainability**: Fix bugs once, all services benefit
- ✅ **Proven Patterns**: Gateway's implementation is battle-tested
- ✅ **Type Safety**: Generic `Cache[T]` interface with compile-time type checking
- ✅ **Future-Proof**: Easy to add new services (Kubernaut Agent, Signal Processing)
- ✅ **Same Effort**: 4-6 hours (same as copy-paste for 4 services)

**Cons**:
- ⚠️ **Refactoring Gateway**: Need to update Gateway to use shared library (1 hour)
- ⚠️ **Breaking Changes**: Future changes to shared library affect all services
  - **Mitigation**: Semantic versioning, backward compatibility

**Effort**:
| Task | Time |
|------|------|
| Extract connection management | 1 hour |
| Extract configuration | 30 minutes |
| Create generic cache interface | 1 hour |
| Refactor Gateway | 1 hour |
| Unit tests | 1 hour |
| Documentation | 30 minutes |
| **Total** | **5 hours** |

**Confidence**: **95%** - Right choice for V1.0, proven patterns

---

### **Alternative B: Copy-Paste Per Service** ❌ **REJECTED**

**Approach**: Each service implements Redis patterns independently

**Pros**:
- ✅ **Service Independence**: Each service owns its Redis code
- ✅ **No Shared Dependencies**: No risk of breaking changes

**Cons**:
- ❌ **Code Duplication**: 4x copies of same code (Gateway, Data Storage, Kubernaut Agent, Signal Processing)
- ❌ **Inconsistency**: Each service might implement differently
- ❌ **Bug Fixes**: Need to fix in 4 places
- ❌ **Technical Debt**: Harder to maintain over time
- ❌ **Same Effort**: 1.5-2 hours × 4 services = 6-8 hours (more than shared library!)

**Effort**:
| Service | Time | Cumulative |
|---------|------|------------|
| Data Storage | 1.5 hours | 1.5 hours |
| Kubernaut Agent | 1.5 hours | 3 hours |
| Signal Processing | 1.5 hours | 4.5 hours |
| Future Service | 1.5 hours | 6 hours |

**Confidence**: **40%** - Not recommended, creates technical debt

---

### **Alternative C: External Library (go-redis-cache)** ❌ **REJECTED**

**Approach**: Use third-party Redis cache library

**Examples**:
- `github.com/go-redis/cache/v9`
- `github.com/eko/gocache`

**Pros**:
- ✅ **No Implementation Needed**: Library already exists
- ✅ **Community Support**: Maintained by community

**Cons**:
- ❌ **External Dependency**: Adds dependency on external library
- ❌ **Not Kubernaut-Specific**: Doesn't include graceful degradation patterns
- ❌ **Learning Curve**: Team needs to learn new library
- ❌ **Gateway Already Works**: Gateway patterns are proven, why change?
- ❌ **Overkill**: We only need simple key-value cache, not advanced features

**Confidence**: **30%** - Not recommended, Gateway patterns are sufficient

---

## 📊 **Decision**

**APPROVED: Alternative A** - Shared Redis Library

**Rationale**:
1. **Break-even at 3 services** - We already have Gateway + Data Storage + Kubernaut Agent (V1.1)
2. **Same effort as copy-paste** - 5 hours vs 6 hours for 4 services
3. **Better maintainability** - Single source of truth
4. **Proven patterns** - Gateway's implementation is battle-tested
5. **Type safety** - Generic `Cache[T]` interface
6. **Future-proof** - Easy to add Signal Processing, Notification services

**Key Insight**: Gateway has already solved all Redis challenges (connection management, graceful degradation, double-checked locking). Extracting into shared library provides same effort as copy-paste but with better maintainability.

---

## 🏗️ **Implementation**

### **Package Structure**

```
pkg/cache/redis/
├── client.go          # Redis client with connection management
├── config.go          # Configuration structure
├── cache.go           # Generic cache interface
├── health.go          # Health check utilities
├── client_test.go     # Client tests
├── cache_test.go      # Cache tests
└── README.md          # Usage documentation
```

### **Core Components**

#### **1. Redis Client** (`client.go`)

**Extracted from**: `pkg/gateway/processing/deduplication.go:109-148`

```go
package redis

import (
    "context"
    "fmt"
    "sync"
    "sync/atomic"

    "github.com/redis/go-redis/v9"
    "go.uber.org/zap"
)

// Client wraps go-redis client with connection management and graceful degradation
type Client struct {
    client      *redis.Client
    logger      *zap.Logger
    connected   atomic.Bool
    connCheckMu sync.Mutex
}

// NewClient creates a new Redis client with connection management
func NewClient(opts *redis.Options, logger *zap.Logger) *Client {
    return &Client{
        client: redis.NewClient(opts),
        logger: logger,
    }
}

// EnsureConnection verifies Redis is available (lazy connection pattern)
// Uses double-checked locking to prevent thundering herd
func (c *Client) EnsureConnection(ctx context.Context) error {
    // Fast path: already connected
    if c.connected.Load() {
        return nil
    }

    // Slow path: need to check connection
    c.connCheckMu.Lock()
    defer c.connCheckMu.Unlock()

    // Double-check after acquiring lock
    if c.connected.Load() {
        return nil
    }

    // Try to connect
    if err := c.client.Ping(ctx).Err(); err != nil {
        return fmt.Errorf("redis unavailable: %w", err)
    }

    // Mark as connected
    c.connected.Store(true)
    c.logger.Info("Redis connection established")
    return nil
}

// GetClient returns the underlying go-redis client
func (c *Client) GetClient() *redis.Client {
    return c.client
}

// Close closes the Redis connection
func (c *Client) Close() error {
    return c.client.Close()
}
```

#### **2. Configuration** (`config.go`)

**Extracted from**: `pkg/gateway/config/config.go:74-83`

```go
package redis

import (
    "time"

    goredis "github.com/redis/go-redis/v9"
)

// Options contains Redis connection configuration
type Options struct {
    Addr         string        `yaml:"addr"`
    DB           int           `yaml:"db"`
    Password     string        `yaml:"password,omitempty"`
    DialTimeout  time.Duration `yaml:"dial_timeout"`
    ReadTimeout  time.Duration `yaml:"read_timeout"`
    WriteTimeout time.Duration `yaml:"write_timeout"`
    PoolSize     int           `yaml:"pool_size"`
    MinIdleConns int           `yaml:"min_idle_conns"`
}

// ToGoRedisOptions converts Options to go-redis Options
func (o *Options) ToGoRedisOptions() *goredis.Options {
    return &goredis.Options{
        Addr:         o.Addr,
        DB:           o.DB,
        Password:     o.Password,
        DialTimeout:  o.DialTimeout,
        ReadTimeout:  o.ReadTimeout,
        WriteTimeout: o.WriteTimeout,
        PoolSize:     o.PoolSize,
        MinIdleConns: o.MinIdleConns,
    }
}
```

#### **3. Generic Cache** (`cache.go`)

**New implementation** (inspired by Gateway's patterns)

```go
package redis

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "time"

    goredis "github.com/redis/go-redis/v9"
    "go.uber.org/zap"
)

// Cache provides a generic key-value cache interface with TTL support
type Cache[T any] struct {
    client *Client
    prefix string
    ttl    time.Duration
}

// NewCache creates a new typed cache
func NewCache[T any](client *Client, prefix string, ttl time.Duration) *Cache[T] {
    return &Cache[T]{
        client: client,
        prefix: prefix,
        ttl:    ttl,
    }
}

// Get retrieves a value from cache
func (c *Cache[T]) Get(ctx context.Context, key string) (*T, error) {
    // Graceful degradation
    if err := c.client.EnsureConnection(ctx); err != nil {
        return nil, fmt.Errorf("redis unavailable: %w", err)
    }

    fullKey := c.prefix + c.hashKey(key)
    data, err := c.client.GetClient().Get(ctx, fullKey).Bytes()
    if err == goredis.Nil {
        return nil, fmt.Errorf("cache miss")
    }
    if err != nil {
        return nil, fmt.Errorf("cache get error: %w", err)
    }

    var value T
    if err := json.Unmarshal(data, &value); err != nil {
        return nil, fmt.Errorf("unmarshal error: %w", err)
    }

    return &value, nil
}

// Set stores a value in cache with TTL
func (c *Cache[T]) Set(ctx context.Context, key string, value *T) error {
    // Graceful degradation
    if err := c.client.EnsureConnection(ctx); err != nil {
        c.client.logger.Warn("Redis unavailable, skipping cache",
            zap.Error(err))
        return nil // Don't fail the request
    }

    fullKey := c.prefix + c.hashKey(key)
    data, err := json.Marshal(value)
    if err != nil {
        return fmt.Errorf("marshal error: %w", err)
    }

    // Use pipeline for atomicity
    pipe := c.client.GetClient().Pipeline()
    pipe.Set(ctx, fullKey, data, 0)
    pipe.Expire(ctx, fullKey, c.ttl)

    if _, err := pipe.Exec(ctx); err != nil {
        c.client.logger.Warn("Redis set failed, skipping cache",
            zap.Error(err))
        return nil // Graceful degradation
    }

    return nil
}

// hashKey creates a deterministic hash of the key
func (c *Cache[T]) hashKey(key string) string {
    hash := sha256.Sum256([]byte(key))
    return hex.EncodeToString(hash[:])
}
```

---

## 📈 **Migration Strategy**

### **Phase 1: Extract Shared Library** (2-3 hours)

1. Create `pkg/cache/redis/` package
2. Extract Gateway's connection management → `client.go`
3. Extract Gateway's config → `config.go`
4. Create generic `Cache[T]` interface → `cache.go`
5. Add unit tests

### **Phase 2: Refactor Gateway** (1 hour)

1. Update Gateway to use `pkg/cache/redis.Client`
2. Remove duplicated code from `pkg/gateway/processing/deduplication.go`
3. Verify integration tests pass

### **Phase 3: Data Storage Integration** (0.5 hours)

1. Use `pkg/cache/redis.Cache[[]float32]` for embedding cache
2. Configure with `prefix: "embedding:"`, `ttl: 24h`

### **Phase 4: Future Services** (0.5 hours each)

1. Kubernaut Agent: Investigation results cache
2. Signal Processing: Pattern detection cache

---

## 📊 **Consequences**

### **Positive**

- ✅ **DRY**: Single source of truth for Redis patterns
- ✅ **Consistency**: All services use same connection management
- ✅ **Maintainability**: Fix bugs once, all services benefit
- ✅ **Type Safety**: Generic `Cache[T]` interface
- ✅ **Proven Patterns**: Gateway's implementation is battle-tested
- ✅ **Future-Proof**: Easy to add new services
- ✅ **Same Effort**: 5 hours vs 6 hours for 4 services

### **Negative**

- ⚠️ **Refactoring Gateway**: Need to update Gateway (1 hour)
  - **Mitigation**: Gateway integration tests verify no regressions
- ⚠️ **Breaking Changes**: Future changes affect all services
  - **Mitigation**: Semantic versioning, backward compatibility
- ⚠️ **Shared Dependency**: All services depend on shared library
  - **Mitigation**: Shared library is internal, controlled by team

### **Neutral**

- 🔄 **Learning Curve**: Team needs to learn shared library API
  - **Mitigation**: Simple API, well-documented, examples provided
- 🔄 **Testing**: Need to test shared library independently
  - **Mitigation**: Unit tests for shared library, integration tests for services

---

## 📊 **Validation Results**

### **ROI Analysis**

**Break-even Point**: 3 services

| Approach | Services | Effort |
|----------|----------|--------|
| **Shared Library** | 1 | 5 hours (one-time) |
| **Copy-Paste** | 3 | 4.5 hours (1.5h × 3) |
| **Copy-Paste** | 4 | 6 hours (1.5h × 4) |

**Current Services**: Gateway + Data Storage + Kubernaut Agent (V1.1) = 3 services
**Future Services**: Signal Processing (V1.1), Notification (V2.0) = 5 services

**Conclusion**: Shared library breaks even at 3 services, saves time at 4+ services

---

## 🔗 **Related Decisions**

- **Builds On**:
  - DD-INFRASTRUCTURE-001 (Gateway/Context-API Redis Separation)
  - DD-INFRASTRUCTURE-002 (Data Storage Redis Strategy)
- **Supports**:
  - DD-EMBEDDING-001 (Embedding Service Implementation)
  - BR-INFRASTRUCTURE-001 (Shared Redis Infrastructure)
- **Enables**:
  - Data Storage embedding cache
  - Kubernaut Agent investigation cache (V1.1)
  - Signal Processing pattern cache (V1.1)

---

## 🔄 **Review & Evolution**

### **When to Revisit**

1. **After 3 months**: Evaluate if shared library is being used correctly
2. **If breaking changes needed**: Consider versioning strategy
3. **If new Redis patterns emerge**: Evaluate if they should be added to shared library
4. **After V1.1 services**: Validate shared library works for Kubernaut Agent, Signal Processing

### **Success Metrics**

| Metric | Target | Actual (TBD) |
|--------|--------|--------------|
| **Services Using Shared Library** | 3+ | TBD |
| **Code Duplication** | 0% | TBD |
| **Bug Fix Propagation** | <1 hour | TBD |
| **Integration Test Pass Rate** | 100% | TBD |

---

## 📚 **References**

- **Gateway Redis Implementation**: `pkg/gateway/processing/deduplication.go`
- **Gateway Configuration**: `pkg/gateway/config/config.go`
- **DD-INFRASTRUCTURE-001**: Gateway/Context-API Redis separation
- **DD-INFRASTRUCTURE-002**: Data Storage Redis strategy
- **DD-EMBEDDING-001**: Embedding service implementation

---

## ✅ **Approval**

**Decision**: Alternative A - Shared Redis Library
**Confidence**: **95%**
**Status**: ✅ **APPROVED** (2025-11-23)

**Rationale**: DRY principle, proven patterns from Gateway, same effort as copy-paste, future-proof for 5+ services. Break-even at 3 services, saves time at 4+ services.

---

**Last Updated**: November 23, 2025
**Next Review**: After Phase 2 completion (Gateway refactored)


