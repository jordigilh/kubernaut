# DD-GATEWAY-005: Redis Cleanup on CRD Deletion

**Status**: ✅ **ACCEPTED** (Option A - No cleanup needed)
**Date**: 2025-10-27
**Deciders**: User, AI Assistant
**Context**: Gateway Integration Tests - Redis cleanup behavior

---

## 🎯 **Decision**

**Do NOT implement automatic Redis fingerprint cleanup when RemediationRequest CRDs are deleted.**

**Current Behavior (KEEP)**:
- Redis deduplication fingerprints expire via TTL (5 minutes default)
- Deleting a RemediationRequest CRD does **not** clean up the Redis fingerprint
- Sending the same alert within 5 minutes will be deduplicated (202 response, no new CRD)

**Rejected Behavior**:
- Automatic cleanup of Redis fingerprints when CRD is deleted
- Immediate recreation of CRD if same alert is sent after CRD deletion

---

## 📋 **Context**

### **Test in Question**
**File**: `test/integration/gateway/redis_integration_test.go:238`
**Test**: `XIt("should clean up Redis state on CRD deletion")`

**Test Expectation** (REJECTED):
1. Send alert → CRD created, Redis fingerprint stored
2. Delete CRD manually
3. Redis fingerprint should be cleaned up immediately
4. Send same alert → New CRD created (not deduplicated)

### **Current Implementation**
**Deduplication Service**:
- Stores fingerprints in Redis with TTL (default: 5 minutes)
- Key format: `dedup:<namespace>:<fingerprint>`
- No cleanup on CRD deletion (TTL-based expiration only)

**CRD Lifecycle**:
- RemediationRequest CRDs may have 24h lifecycle (separate from Redis TTL)
- CRDs can be deleted manually by admins or automatically by controllers
- Redis fingerprints persist for 5 minutes regardless of CRD state

---

## 🤔 **Options Considered**

### **Option A: No Cleanup (Current Behavior)** ✅ **SELECTED**

**Rationale**:
- **Intentional Design**: If an admin deletes a CRD, they likely don't want the same alert to immediately recreate it
- **False Positive Protection**: Deleting a CRD often indicates a false positive or resolved issue
- **Deduplication Window**: 5-minute TTL provides a reasonable "cooldown" period
- **Simplicity**: No additional infrastructure needed (no controller, no K8s API watches)

**Use Cases**:
1. **False Positive**: Admin deletes CRD because alert is incorrect → Same alert shouldn't recreate CRD for 5 minutes
2. **Manual Resolution**: Admin resolves issue and deletes CRD → Same alert shouldn't recreate CRD immediately
3. **Testing**: Developer deletes test CRD → Same alert shouldn't recreate CRD during test cleanup

**Benefits**:
- ✅ Simple implementation (no additional code)
- ✅ Predictable behavior (TTL-based expiration)
- ✅ No K8s API overhead (no CRD watches)
- ✅ Protects against alert storms after CRD deletion

**Drawbacks**:
- ⚠️ 5-minute delay before same alert can create new CRD
- ⚠️ Redis state not immediately synced with K8s state

---

### **Option B: Implement Cleanup** ❌ **REJECTED**

**Rationale**:
- **Immediate Recreation**: If CRD is deleted, same alert can immediately create new CRD
- **State Consistency**: Redis state always synced with K8s state

**Use Cases**:
1. **Reset Alert**: Admin deletes CRD to "reset" alert state → Wants new CRD created immediately
2. **Testing**: Developer wants to test CRD creation multiple times without waiting

**Benefits**:
- ✅ Redis state synced with K8s state
- ✅ Immediate recreation of CRD after deletion

**Drawbacks**:
- ❌ **High Complexity**: Requires controller to watch CRD deletions (4-6 hours implementation)
- ❌ **K8s API Overhead**: Additional watches and API calls
- ❌ **Alert Storm Risk**: Deleting CRD could trigger immediate recreation if alert is still firing
- ❌ **Unclear Business Need**: No documented requirement for this behavior

**Confidence**: 30% (low confidence this is needed)

---

## 📊 **Decision Rationale**

### **Why Option A?**

1. **Intentional Design**: Test comment says *"Current behavior: Redis entries expire via TTL (5 minutes), not immediate cleanup on CRD deletion."* - This suggests intentional design, not a bug.

2. **No Business Requirement**: No documented BR-XXX requirement for immediate Redis cleanup on CRD deletion.

3. **False Positive Protection**: Most common reason to delete a CRD is a false positive or resolved issue. In these cases, you **don't** want the same alert to immediately recreate the CRD.

4. **Simplicity**: Current implementation is simple, predictable, and requires no additional infrastructure.

5. **Low Risk**: 5-minute TTL is a reasonable "cooldown" period. If this becomes a problem, we can:
   - Make TTL configurable (shorter for testing)
   - Implement cleanup in v2.0 if business need arises

### **Confidence Assessment**

**Overall Confidence**: **90%** ✅

**Justification**:
- ✅ Current behavior is intentional (test comment confirms)
- ✅ No business requirement for immediate cleanup
- ✅ False positive protection is valuable
- ✅ Simple implementation (no additional code)
- ⚠️ 10% risk that immediate cleanup is needed for specific use cases

---

## 🔄 **Reversibility**

**If we need to implement cleanup later**:

1. **Trigger**: User reports that 5-minute TTL is blocking legitimate use cases
2. **Implementation**: Create controller to watch RemediationRequest deletions
3. **Effort**: 4-6 hours
4. **Risk**: Low (additive change, doesn't break existing behavior)

**Monitoring**:
- Track user feedback on CRD deletion behavior
- Monitor if 5-minute TTL causes issues in production
- Re-evaluate in v2.0 if business need arises

---

## 📝 **Action Items**

### **Immediate**
- ✅ Delete Test 3 from Redis integration test implementation plan
- ✅ Document decision in DD-GATEWAY-005
- ✅ Update REDIS_TESTS_IMPLEMENTATION_PLAN.md to reflect decision

### **Future (v2.0)**
- ⏸️ Re-evaluate if user feedback indicates need for immediate cleanup
- ⏸️ Consider making TTL configurable (shorter for testing, longer for production)
- ⏸️ Implement cleanup if business requirement emerges

---

## 🔗 **Related Decisions**

- [DD-GATEWAY-001: Deduplication Strategy](./DD-GATEWAY-001-deduplication-strategy.md) (if exists)
- [DD-GATEWAY-002: Redis Fail-Fast Strategy](./DD-GATEWAY-002-redis-fail-fast-strategy.md) (if exists)

---

## 📚 **References**

- **Test File**: `test/integration/gateway/redis_integration_test.go:238`
- **Implementation**: `pkg/gateway/processing/deduplication.go`
- **Business Requirement**: BR-GATEWAY-008 (Deduplication)

---

## 📊 **Impact Assessment**

### **User Impact**
- **Minimal**: Users rarely delete CRDs manually
- **Expected Behavior**: 5-minute cooldown after CRD deletion is reasonable

### **Developer Impact**
- **Testing**: Developers can use configurable TTL (5 seconds for tests)
- **Integration Tests**: Tests can wait 6 seconds for TTL expiration

### **Operations Impact**
- **Monitoring**: No additional monitoring needed
- **Troubleshooting**: Clear behavior (TTL-based expiration)

---

## 🎯 **Success Criteria**

**How we'll know this decision is correct**:
1. ✅ No user complaints about 5-minute TTL blocking legitimate use cases
2. ✅ Integration tests pass with configurable TTL
3. ✅ False positive protection works as expected
4. ✅ Simple implementation reduces maintenance burden

**How we'll know we need to revisit**:
1. ❌ Users report that 5-minute TTL blocks legitimate use cases
2. ❌ Frequent requests to "reset" alerts by deleting CRDs
3. ❌ Testing workflows blocked by 5-minute delay

---

**Status**: ✅ **DECISION ACCEPTED**
**Next Review**: v2.0 planning (or if user feedback indicates need)
**Owner**: Gateway Service Team



**Status**: ✅ **ACCEPTED** (Option A - No cleanup needed)
**Date**: 2025-10-27
**Deciders**: User, AI Assistant
**Context**: Gateway Integration Tests - Redis cleanup behavior

---

## 🎯 **Decision**

**Do NOT implement automatic Redis fingerprint cleanup when RemediationRequest CRDs are deleted.**

**Current Behavior (KEEP)**:
- Redis deduplication fingerprints expire via TTL (5 minutes default)
- Deleting a RemediationRequest CRD does **not** clean up the Redis fingerprint
- Sending the same alert within 5 minutes will be deduplicated (202 response, no new CRD)

**Rejected Behavior**:
- Automatic cleanup of Redis fingerprints when CRD is deleted
- Immediate recreation of CRD if same alert is sent after CRD deletion

---

## 📋 **Context**

### **Test in Question**
**File**: `test/integration/gateway/redis_integration_test.go:238`
**Test**: `XIt("should clean up Redis state on CRD deletion")`

**Test Expectation** (REJECTED):
1. Send alert → CRD created, Redis fingerprint stored
2. Delete CRD manually
3. Redis fingerprint should be cleaned up immediately
4. Send same alert → New CRD created (not deduplicated)

### **Current Implementation**
**Deduplication Service**:
- Stores fingerprints in Redis with TTL (default: 5 minutes)
- Key format: `dedup:<namespace>:<fingerprint>`
- No cleanup on CRD deletion (TTL-based expiration only)

**CRD Lifecycle**:
- RemediationRequest CRDs may have 24h lifecycle (separate from Redis TTL)
- CRDs can be deleted manually by admins or automatically by controllers
- Redis fingerprints persist for 5 minutes regardless of CRD state

---

## 🤔 **Options Considered**

### **Option A: No Cleanup (Current Behavior)** ✅ **SELECTED**

**Rationale**:
- **Intentional Design**: If an admin deletes a CRD, they likely don't want the same alert to immediately recreate it
- **False Positive Protection**: Deleting a CRD often indicates a false positive or resolved issue
- **Deduplication Window**: 5-minute TTL provides a reasonable "cooldown" period
- **Simplicity**: No additional infrastructure needed (no controller, no K8s API watches)

**Use Cases**:
1. **False Positive**: Admin deletes CRD because alert is incorrect → Same alert shouldn't recreate CRD for 5 minutes
2. **Manual Resolution**: Admin resolves issue and deletes CRD → Same alert shouldn't recreate CRD immediately
3. **Testing**: Developer deletes test CRD → Same alert shouldn't recreate CRD during test cleanup

**Benefits**:
- ✅ Simple implementation (no additional code)
- ✅ Predictable behavior (TTL-based expiration)
- ✅ No K8s API overhead (no CRD watches)
- ✅ Protects against alert storms after CRD deletion

**Drawbacks**:
- ⚠️ 5-minute delay before same alert can create new CRD
- ⚠️ Redis state not immediately synced with K8s state

---

### **Option B: Implement Cleanup** ❌ **REJECTED**

**Rationale**:
- **Immediate Recreation**: If CRD is deleted, same alert can immediately create new CRD
- **State Consistency**: Redis state always synced with K8s state

**Use Cases**:
1. **Reset Alert**: Admin deletes CRD to "reset" alert state → Wants new CRD created immediately
2. **Testing**: Developer wants to test CRD creation multiple times without waiting

**Benefits**:
- ✅ Redis state synced with K8s state
- ✅ Immediate recreation of CRD after deletion

**Drawbacks**:
- ❌ **High Complexity**: Requires controller to watch CRD deletions (4-6 hours implementation)
- ❌ **K8s API Overhead**: Additional watches and API calls
- ❌ **Alert Storm Risk**: Deleting CRD could trigger immediate recreation if alert is still firing
- ❌ **Unclear Business Need**: No documented requirement for this behavior

**Confidence**: 30% (low confidence this is needed)

---

## 📊 **Decision Rationale**

### **Why Option A?**

1. **Intentional Design**: Test comment says *"Current behavior: Redis entries expire via TTL (5 minutes), not immediate cleanup on CRD deletion."* - This suggests intentional design, not a bug.

2. **No Business Requirement**: No documented BR-XXX requirement for immediate Redis cleanup on CRD deletion.

3. **False Positive Protection**: Most common reason to delete a CRD is a false positive or resolved issue. In these cases, you **don't** want the same alert to immediately recreate the CRD.

4. **Simplicity**: Current implementation is simple, predictable, and requires no additional infrastructure.

5. **Low Risk**: 5-minute TTL is a reasonable "cooldown" period. If this becomes a problem, we can:
   - Make TTL configurable (shorter for testing)
   - Implement cleanup in v2.0 if business need arises

### **Confidence Assessment**

**Overall Confidence**: **90%** ✅

**Justification**:
- ✅ Current behavior is intentional (test comment confirms)
- ✅ No business requirement for immediate cleanup
- ✅ False positive protection is valuable
- ✅ Simple implementation (no additional code)
- ⚠️ 10% risk that immediate cleanup is needed for specific use cases

---

## 🔄 **Reversibility**

**If we need to implement cleanup later**:

1. **Trigger**: User reports that 5-minute TTL is blocking legitimate use cases
2. **Implementation**: Create controller to watch RemediationRequest deletions
3. **Effort**: 4-6 hours
4. **Risk**: Low (additive change, doesn't break existing behavior)

**Monitoring**:
- Track user feedback on CRD deletion behavior
- Monitor if 5-minute TTL causes issues in production
- Re-evaluate in v2.0 if business need arises

---

## 📝 **Action Items**

### **Immediate**
- ✅ Delete Test 3 from Redis integration test implementation plan
- ✅ Document decision in DD-GATEWAY-005
- ✅ Update REDIS_TESTS_IMPLEMENTATION_PLAN.md to reflect decision

### **Future (v2.0)**
- ⏸️ Re-evaluate if user feedback indicates need for immediate cleanup
- ⏸️ Consider making TTL configurable (shorter for testing, longer for production)
- ⏸️ Implement cleanup if business requirement emerges

---

## 🔗 **Related Decisions**

- [DD-GATEWAY-001: Deduplication Strategy](./DD-GATEWAY-001-deduplication-strategy.md) (if exists)
- [DD-GATEWAY-002: Redis Fail-Fast Strategy](./DD-GATEWAY-002-redis-fail-fast-strategy.md) (if exists)

---

## 📚 **References**

- **Test File**: `test/integration/gateway/redis_integration_test.go:238`
- **Implementation**: `pkg/gateway/processing/deduplication.go`
- **Business Requirement**: BR-GATEWAY-008 (Deduplication)

---

## 📊 **Impact Assessment**

### **User Impact**
- **Minimal**: Users rarely delete CRDs manually
- **Expected Behavior**: 5-minute cooldown after CRD deletion is reasonable

### **Developer Impact**
- **Testing**: Developers can use configurable TTL (5 seconds for tests)
- **Integration Tests**: Tests can wait 6 seconds for TTL expiration

### **Operations Impact**
- **Monitoring**: No additional monitoring needed
- **Troubleshooting**: Clear behavior (TTL-based expiration)

---

## 🎯 **Success Criteria**

**How we'll know this decision is correct**:
1. ✅ No user complaints about 5-minute TTL blocking legitimate use cases
2. ✅ Integration tests pass with configurable TTL
3. ✅ False positive protection works as expected
4. ✅ Simple implementation reduces maintenance burden

**How we'll know we need to revisit**:
1. ❌ Users report that 5-minute TTL blocks legitimate use cases
2. ❌ Frequent requests to "reset" alerts by deleting CRDs
3. ❌ Testing workflows blocked by 5-minute delay

---

**Status**: ✅ **DECISION ACCEPTED**
**Next Review**: v2.0 planning (or if user feedback indicates need)
**Owner**: Gateway Service Team

# DD-GATEWAY-005: Redis Cleanup on CRD Deletion

**Status**: ✅ **ACCEPTED** (Option A - No cleanup needed)
**Date**: 2025-10-27
**Deciders**: User, AI Assistant
**Context**: Gateway Integration Tests - Redis cleanup behavior

---

## 🎯 **Decision**

**Do NOT implement automatic Redis fingerprint cleanup when RemediationRequest CRDs are deleted.**

**Current Behavior (KEEP)**:
- Redis deduplication fingerprints expire via TTL (5 minutes default)
- Deleting a RemediationRequest CRD does **not** clean up the Redis fingerprint
- Sending the same alert within 5 minutes will be deduplicated (202 response, no new CRD)

**Rejected Behavior**:
- Automatic cleanup of Redis fingerprints when CRD is deleted
- Immediate recreation of CRD if same alert is sent after CRD deletion

---

## 📋 **Context**

### **Test in Question**
**File**: `test/integration/gateway/redis_integration_test.go:238`
**Test**: `XIt("should clean up Redis state on CRD deletion")`

**Test Expectation** (REJECTED):
1. Send alert → CRD created, Redis fingerprint stored
2. Delete CRD manually
3. Redis fingerprint should be cleaned up immediately
4. Send same alert → New CRD created (not deduplicated)

### **Current Implementation**
**Deduplication Service**:
- Stores fingerprints in Redis with TTL (default: 5 minutes)
- Key format: `dedup:<namespace>:<fingerprint>`
- No cleanup on CRD deletion (TTL-based expiration only)

**CRD Lifecycle**:
- RemediationRequest CRDs may have 24h lifecycle (separate from Redis TTL)
- CRDs can be deleted manually by admins or automatically by controllers
- Redis fingerprints persist for 5 minutes regardless of CRD state

---

## 🤔 **Options Considered**

### **Option A: No Cleanup (Current Behavior)** ✅ **SELECTED**

**Rationale**:
- **Intentional Design**: If an admin deletes a CRD, they likely don't want the same alert to immediately recreate it
- **False Positive Protection**: Deleting a CRD often indicates a false positive or resolved issue
- **Deduplication Window**: 5-minute TTL provides a reasonable "cooldown" period
- **Simplicity**: No additional infrastructure needed (no controller, no K8s API watches)

**Use Cases**:
1. **False Positive**: Admin deletes CRD because alert is incorrect → Same alert shouldn't recreate CRD for 5 minutes
2. **Manual Resolution**: Admin resolves issue and deletes CRD → Same alert shouldn't recreate CRD immediately
3. **Testing**: Developer deletes test CRD → Same alert shouldn't recreate CRD during test cleanup

**Benefits**:
- ✅ Simple implementation (no additional code)
- ✅ Predictable behavior (TTL-based expiration)
- ✅ No K8s API overhead (no CRD watches)
- ✅ Protects against alert storms after CRD deletion

**Drawbacks**:
- ⚠️ 5-minute delay before same alert can create new CRD
- ⚠️ Redis state not immediately synced with K8s state

---

### **Option B: Implement Cleanup** ❌ **REJECTED**

**Rationale**:
- **Immediate Recreation**: If CRD is deleted, same alert can immediately create new CRD
- **State Consistency**: Redis state always synced with K8s state

**Use Cases**:
1. **Reset Alert**: Admin deletes CRD to "reset" alert state → Wants new CRD created immediately
2. **Testing**: Developer wants to test CRD creation multiple times without waiting

**Benefits**:
- ✅ Redis state synced with K8s state
- ✅ Immediate recreation of CRD after deletion

**Drawbacks**:
- ❌ **High Complexity**: Requires controller to watch CRD deletions (4-6 hours implementation)
- ❌ **K8s API Overhead**: Additional watches and API calls
- ❌ **Alert Storm Risk**: Deleting CRD could trigger immediate recreation if alert is still firing
- ❌ **Unclear Business Need**: No documented requirement for this behavior

**Confidence**: 30% (low confidence this is needed)

---

## 📊 **Decision Rationale**

### **Why Option A?**

1. **Intentional Design**: Test comment says *"Current behavior: Redis entries expire via TTL (5 minutes), not immediate cleanup on CRD deletion."* - This suggests intentional design, not a bug.

2. **No Business Requirement**: No documented BR-XXX requirement for immediate Redis cleanup on CRD deletion.

3. **False Positive Protection**: Most common reason to delete a CRD is a false positive or resolved issue. In these cases, you **don't** want the same alert to immediately recreate the CRD.

4. **Simplicity**: Current implementation is simple, predictable, and requires no additional infrastructure.

5. **Low Risk**: 5-minute TTL is a reasonable "cooldown" period. If this becomes a problem, we can:
   - Make TTL configurable (shorter for testing)
   - Implement cleanup in v2.0 if business need arises

### **Confidence Assessment**

**Overall Confidence**: **90%** ✅

**Justification**:
- ✅ Current behavior is intentional (test comment confirms)
- ✅ No business requirement for immediate cleanup
- ✅ False positive protection is valuable
- ✅ Simple implementation (no additional code)
- ⚠️ 10% risk that immediate cleanup is needed for specific use cases

---

## 🔄 **Reversibility**

**If we need to implement cleanup later**:

1. **Trigger**: User reports that 5-minute TTL is blocking legitimate use cases
2. **Implementation**: Create controller to watch RemediationRequest deletions
3. **Effort**: 4-6 hours
4. **Risk**: Low (additive change, doesn't break existing behavior)

**Monitoring**:
- Track user feedback on CRD deletion behavior
- Monitor if 5-minute TTL causes issues in production
- Re-evaluate in v2.0 if business need arises

---

## 📝 **Action Items**

### **Immediate**
- ✅ Delete Test 3 from Redis integration test implementation plan
- ✅ Document decision in DD-GATEWAY-005
- ✅ Update REDIS_TESTS_IMPLEMENTATION_PLAN.md to reflect decision

### **Future (v2.0)**
- ⏸️ Re-evaluate if user feedback indicates need for immediate cleanup
- ⏸️ Consider making TTL configurable (shorter for testing, longer for production)
- ⏸️ Implement cleanup if business requirement emerges

---

## 🔗 **Related Decisions**

- [DD-GATEWAY-001: Deduplication Strategy](./DD-GATEWAY-001-deduplication-strategy.md) (if exists)
- [DD-GATEWAY-002: Redis Fail-Fast Strategy](./DD-GATEWAY-002-redis-fail-fast-strategy.md) (if exists)

---

## 📚 **References**

- **Test File**: `test/integration/gateway/redis_integration_test.go:238`
- **Implementation**: `pkg/gateway/processing/deduplication.go`
- **Business Requirement**: BR-GATEWAY-008 (Deduplication)

---

## 📊 **Impact Assessment**

### **User Impact**
- **Minimal**: Users rarely delete CRDs manually
- **Expected Behavior**: 5-minute cooldown after CRD deletion is reasonable

### **Developer Impact**
- **Testing**: Developers can use configurable TTL (5 seconds for tests)
- **Integration Tests**: Tests can wait 6 seconds for TTL expiration

### **Operations Impact**
- **Monitoring**: No additional monitoring needed
- **Troubleshooting**: Clear behavior (TTL-based expiration)

---

## 🎯 **Success Criteria**

**How we'll know this decision is correct**:
1. ✅ No user complaints about 5-minute TTL blocking legitimate use cases
2. ✅ Integration tests pass with configurable TTL
3. ✅ False positive protection works as expected
4. ✅ Simple implementation reduces maintenance burden

**How we'll know we need to revisit**:
1. ❌ Users report that 5-minute TTL blocks legitimate use cases
2. ❌ Frequent requests to "reset" alerts by deleting CRDs
3. ❌ Testing workflows blocked by 5-minute delay

---

**Status**: ✅ **DECISION ACCEPTED**
**Next Review**: v2.0 planning (or if user feedback indicates need)
**Owner**: Gateway Service Team

# DD-GATEWAY-005: Redis Cleanup on CRD Deletion

**Status**: ✅ **ACCEPTED** (Option A - No cleanup needed)
**Date**: 2025-10-27
**Deciders**: User, AI Assistant
**Context**: Gateway Integration Tests - Redis cleanup behavior

---

## 🎯 **Decision**

**Do NOT implement automatic Redis fingerprint cleanup when RemediationRequest CRDs are deleted.**

**Current Behavior (KEEP)**:
- Redis deduplication fingerprints expire via TTL (5 minutes default)
- Deleting a RemediationRequest CRD does **not** clean up the Redis fingerprint
- Sending the same alert within 5 minutes will be deduplicated (202 response, no new CRD)

**Rejected Behavior**:
- Automatic cleanup of Redis fingerprints when CRD is deleted
- Immediate recreation of CRD if same alert is sent after CRD deletion

---

## 📋 **Context**

### **Test in Question**
**File**: `test/integration/gateway/redis_integration_test.go:238`
**Test**: `XIt("should clean up Redis state on CRD deletion")`

**Test Expectation** (REJECTED):
1. Send alert → CRD created, Redis fingerprint stored
2. Delete CRD manually
3. Redis fingerprint should be cleaned up immediately
4. Send same alert → New CRD created (not deduplicated)

### **Current Implementation**
**Deduplication Service**:
- Stores fingerprints in Redis with TTL (default: 5 minutes)
- Key format: `dedup:<namespace>:<fingerprint>`
- No cleanup on CRD deletion (TTL-based expiration only)

**CRD Lifecycle**:
- RemediationRequest CRDs may have 24h lifecycle (separate from Redis TTL)
- CRDs can be deleted manually by admins or automatically by controllers
- Redis fingerprints persist for 5 minutes regardless of CRD state

---

## 🤔 **Options Considered**

### **Option A: No Cleanup (Current Behavior)** ✅ **SELECTED**

**Rationale**:
- **Intentional Design**: If an admin deletes a CRD, they likely don't want the same alert to immediately recreate it
- **False Positive Protection**: Deleting a CRD often indicates a false positive or resolved issue
- **Deduplication Window**: 5-minute TTL provides a reasonable "cooldown" period
- **Simplicity**: No additional infrastructure needed (no controller, no K8s API watches)

**Use Cases**:
1. **False Positive**: Admin deletes CRD because alert is incorrect → Same alert shouldn't recreate CRD for 5 minutes
2. **Manual Resolution**: Admin resolves issue and deletes CRD → Same alert shouldn't recreate CRD immediately
3. **Testing**: Developer deletes test CRD → Same alert shouldn't recreate CRD during test cleanup

**Benefits**:
- ✅ Simple implementation (no additional code)
- ✅ Predictable behavior (TTL-based expiration)
- ✅ No K8s API overhead (no CRD watches)
- ✅ Protects against alert storms after CRD deletion

**Drawbacks**:
- ⚠️ 5-minute delay before same alert can create new CRD
- ⚠️ Redis state not immediately synced with K8s state

---

### **Option B: Implement Cleanup** ❌ **REJECTED**

**Rationale**:
- **Immediate Recreation**: If CRD is deleted, same alert can immediately create new CRD
- **State Consistency**: Redis state always synced with K8s state

**Use Cases**:
1. **Reset Alert**: Admin deletes CRD to "reset" alert state → Wants new CRD created immediately
2. **Testing**: Developer wants to test CRD creation multiple times without waiting

**Benefits**:
- ✅ Redis state synced with K8s state
- ✅ Immediate recreation of CRD after deletion

**Drawbacks**:
- ❌ **High Complexity**: Requires controller to watch CRD deletions (4-6 hours implementation)
- ❌ **K8s API Overhead**: Additional watches and API calls
- ❌ **Alert Storm Risk**: Deleting CRD could trigger immediate recreation if alert is still firing
- ❌ **Unclear Business Need**: No documented requirement for this behavior

**Confidence**: 30% (low confidence this is needed)

---

## 📊 **Decision Rationale**

### **Why Option A?**

1. **Intentional Design**: Test comment says *"Current behavior: Redis entries expire via TTL (5 minutes), not immediate cleanup on CRD deletion."* - This suggests intentional design, not a bug.

2. **No Business Requirement**: No documented BR-XXX requirement for immediate Redis cleanup on CRD deletion.

3. **False Positive Protection**: Most common reason to delete a CRD is a false positive or resolved issue. In these cases, you **don't** want the same alert to immediately recreate the CRD.

4. **Simplicity**: Current implementation is simple, predictable, and requires no additional infrastructure.

5. **Low Risk**: 5-minute TTL is a reasonable "cooldown" period. If this becomes a problem, we can:
   - Make TTL configurable (shorter for testing)
   - Implement cleanup in v2.0 if business need arises

### **Confidence Assessment**

**Overall Confidence**: **90%** ✅

**Justification**:
- ✅ Current behavior is intentional (test comment confirms)
- ✅ No business requirement for immediate cleanup
- ✅ False positive protection is valuable
- ✅ Simple implementation (no additional code)
- ⚠️ 10% risk that immediate cleanup is needed for specific use cases

---

## 🔄 **Reversibility**

**If we need to implement cleanup later**:

1. **Trigger**: User reports that 5-minute TTL is blocking legitimate use cases
2. **Implementation**: Create controller to watch RemediationRequest deletions
3. **Effort**: 4-6 hours
4. **Risk**: Low (additive change, doesn't break existing behavior)

**Monitoring**:
- Track user feedback on CRD deletion behavior
- Monitor if 5-minute TTL causes issues in production
- Re-evaluate in v2.0 if business need arises

---

## 📝 **Action Items**

### **Immediate**
- ✅ Delete Test 3 from Redis integration test implementation plan
- ✅ Document decision in DD-GATEWAY-005
- ✅ Update REDIS_TESTS_IMPLEMENTATION_PLAN.md to reflect decision

### **Future (v2.0)**
- ⏸️ Re-evaluate if user feedback indicates need for immediate cleanup
- ⏸️ Consider making TTL configurable (shorter for testing, longer for production)
- ⏸️ Implement cleanup if business requirement emerges

---

## 🔗 **Related Decisions**

- [DD-GATEWAY-001: Deduplication Strategy](./DD-GATEWAY-001-deduplication-strategy.md) (if exists)
- [DD-GATEWAY-002: Redis Fail-Fast Strategy](./DD-GATEWAY-002-redis-fail-fast-strategy.md) (if exists)

---

## 📚 **References**

- **Test File**: `test/integration/gateway/redis_integration_test.go:238`
- **Implementation**: `pkg/gateway/processing/deduplication.go`
- **Business Requirement**: BR-GATEWAY-008 (Deduplication)

---

## 📊 **Impact Assessment**

### **User Impact**
- **Minimal**: Users rarely delete CRDs manually
- **Expected Behavior**: 5-minute cooldown after CRD deletion is reasonable

### **Developer Impact**
- **Testing**: Developers can use configurable TTL (5 seconds for tests)
- **Integration Tests**: Tests can wait 6 seconds for TTL expiration

### **Operations Impact**
- **Monitoring**: No additional monitoring needed
- **Troubleshooting**: Clear behavior (TTL-based expiration)

---

## 🎯 **Success Criteria**

**How we'll know this decision is correct**:
1. ✅ No user complaints about 5-minute TTL blocking legitimate use cases
2. ✅ Integration tests pass with configurable TTL
3. ✅ False positive protection works as expected
4. ✅ Simple implementation reduces maintenance burden

**How we'll know we need to revisit**:
1. ❌ Users report that 5-minute TTL blocks legitimate use cases
2. ❌ Frequent requests to "reset" alerts by deleting CRDs
3. ❌ Testing workflows blocked by 5-minute delay

---

**Status**: ✅ **DECISION ACCEPTED**
**Next Review**: v2.0 planning (or if user feedback indicates need)
**Owner**: Gateway Service Team



**Status**: ✅ **ACCEPTED** (Option A - No cleanup needed)
**Date**: 2025-10-27
**Deciders**: User, AI Assistant
**Context**: Gateway Integration Tests - Redis cleanup behavior

---

## 🎯 **Decision**

**Do NOT implement automatic Redis fingerprint cleanup when RemediationRequest CRDs are deleted.**

**Current Behavior (KEEP)**:
- Redis deduplication fingerprints expire via TTL (5 minutes default)
- Deleting a RemediationRequest CRD does **not** clean up the Redis fingerprint
- Sending the same alert within 5 minutes will be deduplicated (202 response, no new CRD)

**Rejected Behavior**:
- Automatic cleanup of Redis fingerprints when CRD is deleted
- Immediate recreation of CRD if same alert is sent after CRD deletion

---

## 📋 **Context**

### **Test in Question**
**File**: `test/integration/gateway/redis_integration_test.go:238`
**Test**: `XIt("should clean up Redis state on CRD deletion")`

**Test Expectation** (REJECTED):
1. Send alert → CRD created, Redis fingerprint stored
2. Delete CRD manually
3. Redis fingerprint should be cleaned up immediately
4. Send same alert → New CRD created (not deduplicated)

### **Current Implementation**
**Deduplication Service**:
- Stores fingerprints in Redis with TTL (default: 5 minutes)
- Key format: `dedup:<namespace>:<fingerprint>`
- No cleanup on CRD deletion (TTL-based expiration only)

**CRD Lifecycle**:
- RemediationRequest CRDs may have 24h lifecycle (separate from Redis TTL)
- CRDs can be deleted manually by admins or automatically by controllers
- Redis fingerprints persist for 5 minutes regardless of CRD state

---

## 🤔 **Options Considered**

### **Option A: No Cleanup (Current Behavior)** ✅ **SELECTED**

**Rationale**:
- **Intentional Design**: If an admin deletes a CRD, they likely don't want the same alert to immediately recreate it
- **False Positive Protection**: Deleting a CRD often indicates a false positive or resolved issue
- **Deduplication Window**: 5-minute TTL provides a reasonable "cooldown" period
- **Simplicity**: No additional infrastructure needed (no controller, no K8s API watches)

**Use Cases**:
1. **False Positive**: Admin deletes CRD because alert is incorrect → Same alert shouldn't recreate CRD for 5 minutes
2. **Manual Resolution**: Admin resolves issue and deletes CRD → Same alert shouldn't recreate CRD immediately
3. **Testing**: Developer deletes test CRD → Same alert shouldn't recreate CRD during test cleanup

**Benefits**:
- ✅ Simple implementation (no additional code)
- ✅ Predictable behavior (TTL-based expiration)
- ✅ No K8s API overhead (no CRD watches)
- ✅ Protects against alert storms after CRD deletion

**Drawbacks**:
- ⚠️ 5-minute delay before same alert can create new CRD
- ⚠️ Redis state not immediately synced with K8s state

---

### **Option B: Implement Cleanup** ❌ **REJECTED**

**Rationale**:
- **Immediate Recreation**: If CRD is deleted, same alert can immediately create new CRD
- **State Consistency**: Redis state always synced with K8s state

**Use Cases**:
1. **Reset Alert**: Admin deletes CRD to "reset" alert state → Wants new CRD created immediately
2. **Testing**: Developer wants to test CRD creation multiple times without waiting

**Benefits**:
- ✅ Redis state synced with K8s state
- ✅ Immediate recreation of CRD after deletion

**Drawbacks**:
- ❌ **High Complexity**: Requires controller to watch CRD deletions (4-6 hours implementation)
- ❌ **K8s API Overhead**: Additional watches and API calls
- ❌ **Alert Storm Risk**: Deleting CRD could trigger immediate recreation if alert is still firing
- ❌ **Unclear Business Need**: No documented requirement for this behavior

**Confidence**: 30% (low confidence this is needed)

---

## 📊 **Decision Rationale**

### **Why Option A?**

1. **Intentional Design**: Test comment says *"Current behavior: Redis entries expire via TTL (5 minutes), not immediate cleanup on CRD deletion."* - This suggests intentional design, not a bug.

2. **No Business Requirement**: No documented BR-XXX requirement for immediate Redis cleanup on CRD deletion.

3. **False Positive Protection**: Most common reason to delete a CRD is a false positive or resolved issue. In these cases, you **don't** want the same alert to immediately recreate the CRD.

4. **Simplicity**: Current implementation is simple, predictable, and requires no additional infrastructure.

5. **Low Risk**: 5-minute TTL is a reasonable "cooldown" period. If this becomes a problem, we can:
   - Make TTL configurable (shorter for testing)
   - Implement cleanup in v2.0 if business need arises

### **Confidence Assessment**

**Overall Confidence**: **90%** ✅

**Justification**:
- ✅ Current behavior is intentional (test comment confirms)
- ✅ No business requirement for immediate cleanup
- ✅ False positive protection is valuable
- ✅ Simple implementation (no additional code)
- ⚠️ 10% risk that immediate cleanup is needed for specific use cases

---

## 🔄 **Reversibility**

**If we need to implement cleanup later**:

1. **Trigger**: User reports that 5-minute TTL is blocking legitimate use cases
2. **Implementation**: Create controller to watch RemediationRequest deletions
3. **Effort**: 4-6 hours
4. **Risk**: Low (additive change, doesn't break existing behavior)

**Monitoring**:
- Track user feedback on CRD deletion behavior
- Monitor if 5-minute TTL causes issues in production
- Re-evaluate in v2.0 if business need arises

---

## 📝 **Action Items**

### **Immediate**
- ✅ Delete Test 3 from Redis integration test implementation plan
- ✅ Document decision in DD-GATEWAY-005
- ✅ Update REDIS_TESTS_IMPLEMENTATION_PLAN.md to reflect decision

### **Future (v2.0)**
- ⏸️ Re-evaluate if user feedback indicates need for immediate cleanup
- ⏸️ Consider making TTL configurable (shorter for testing, longer for production)
- ⏸️ Implement cleanup if business requirement emerges

---

## 🔗 **Related Decisions**

- [DD-GATEWAY-001: Deduplication Strategy](./DD-GATEWAY-001-deduplication-strategy.md) (if exists)
- [DD-GATEWAY-002: Redis Fail-Fast Strategy](./DD-GATEWAY-002-redis-fail-fast-strategy.md) (if exists)

---

## 📚 **References**

- **Test File**: `test/integration/gateway/redis_integration_test.go:238`
- **Implementation**: `pkg/gateway/processing/deduplication.go`
- **Business Requirement**: BR-GATEWAY-008 (Deduplication)

---

## 📊 **Impact Assessment**

### **User Impact**
- **Minimal**: Users rarely delete CRDs manually
- **Expected Behavior**: 5-minute cooldown after CRD deletion is reasonable

### **Developer Impact**
- **Testing**: Developers can use configurable TTL (5 seconds for tests)
- **Integration Tests**: Tests can wait 6 seconds for TTL expiration

### **Operations Impact**
- **Monitoring**: No additional monitoring needed
- **Troubleshooting**: Clear behavior (TTL-based expiration)

---

## 🎯 **Success Criteria**

**How we'll know this decision is correct**:
1. ✅ No user complaints about 5-minute TTL blocking legitimate use cases
2. ✅ Integration tests pass with configurable TTL
3. ✅ False positive protection works as expected
4. ✅ Simple implementation reduces maintenance burden

**How we'll know we need to revisit**:
1. ❌ Users report that 5-minute TTL blocks legitimate use cases
2. ❌ Frequent requests to "reset" alerts by deleting CRDs
3. ❌ Testing workflows blocked by 5-minute delay

---

**Status**: ✅ **DECISION ACCEPTED**
**Next Review**: v2.0 planning (or if user feedback indicates need)
**Owner**: Gateway Service Team

# DD-GATEWAY-005: Redis Cleanup on CRD Deletion

**Status**: ✅ **ACCEPTED** (Option A - No cleanup needed)
**Date**: 2025-10-27
**Deciders**: User, AI Assistant
**Context**: Gateway Integration Tests - Redis cleanup behavior

---

## 🎯 **Decision**

**Do NOT implement automatic Redis fingerprint cleanup when RemediationRequest CRDs are deleted.**

**Current Behavior (KEEP)**:
- Redis deduplication fingerprints expire via TTL (5 minutes default)
- Deleting a RemediationRequest CRD does **not** clean up the Redis fingerprint
- Sending the same alert within 5 minutes will be deduplicated (202 response, no new CRD)

**Rejected Behavior**:
- Automatic cleanup of Redis fingerprints when CRD is deleted
- Immediate recreation of CRD if same alert is sent after CRD deletion

---

## 📋 **Context**

### **Test in Question**
**File**: `test/integration/gateway/redis_integration_test.go:238`
**Test**: `XIt("should clean up Redis state on CRD deletion")`

**Test Expectation** (REJECTED):
1. Send alert → CRD created, Redis fingerprint stored
2. Delete CRD manually
3. Redis fingerprint should be cleaned up immediately
4. Send same alert → New CRD created (not deduplicated)

### **Current Implementation**
**Deduplication Service**:
- Stores fingerprints in Redis with TTL (default: 5 minutes)
- Key format: `dedup:<namespace>:<fingerprint>`
- No cleanup on CRD deletion (TTL-based expiration only)

**CRD Lifecycle**:
- RemediationRequest CRDs may have 24h lifecycle (separate from Redis TTL)
- CRDs can be deleted manually by admins or automatically by controllers
- Redis fingerprints persist for 5 minutes regardless of CRD state

---

## 🤔 **Options Considered**

### **Option A: No Cleanup (Current Behavior)** ✅ **SELECTED**

**Rationale**:
- **Intentional Design**: If an admin deletes a CRD, they likely don't want the same alert to immediately recreate it
- **False Positive Protection**: Deleting a CRD often indicates a false positive or resolved issue
- **Deduplication Window**: 5-minute TTL provides a reasonable "cooldown" period
- **Simplicity**: No additional infrastructure needed (no controller, no K8s API watches)

**Use Cases**:
1. **False Positive**: Admin deletes CRD because alert is incorrect → Same alert shouldn't recreate CRD for 5 minutes
2. **Manual Resolution**: Admin resolves issue and deletes CRD → Same alert shouldn't recreate CRD immediately
3. **Testing**: Developer deletes test CRD → Same alert shouldn't recreate CRD during test cleanup

**Benefits**:
- ✅ Simple implementation (no additional code)
- ✅ Predictable behavior (TTL-based expiration)
- ✅ No K8s API overhead (no CRD watches)
- ✅ Protects against alert storms after CRD deletion

**Drawbacks**:
- ⚠️ 5-minute delay before same alert can create new CRD
- ⚠️ Redis state not immediately synced with K8s state

---

### **Option B: Implement Cleanup** ❌ **REJECTED**

**Rationale**:
- **Immediate Recreation**: If CRD is deleted, same alert can immediately create new CRD
- **State Consistency**: Redis state always synced with K8s state

**Use Cases**:
1. **Reset Alert**: Admin deletes CRD to "reset" alert state → Wants new CRD created immediately
2. **Testing**: Developer wants to test CRD creation multiple times without waiting

**Benefits**:
- ✅ Redis state synced with K8s state
- ✅ Immediate recreation of CRD after deletion

**Drawbacks**:
- ❌ **High Complexity**: Requires controller to watch CRD deletions (4-6 hours implementation)
- ❌ **K8s API Overhead**: Additional watches and API calls
- ❌ **Alert Storm Risk**: Deleting CRD could trigger immediate recreation if alert is still firing
- ❌ **Unclear Business Need**: No documented requirement for this behavior

**Confidence**: 30% (low confidence this is needed)

---

## 📊 **Decision Rationale**

### **Why Option A?**

1. **Intentional Design**: Test comment says *"Current behavior: Redis entries expire via TTL (5 minutes), not immediate cleanup on CRD deletion."* - This suggests intentional design, not a bug.

2. **No Business Requirement**: No documented BR-XXX requirement for immediate Redis cleanup on CRD deletion.

3. **False Positive Protection**: Most common reason to delete a CRD is a false positive or resolved issue. In these cases, you **don't** want the same alert to immediately recreate the CRD.

4. **Simplicity**: Current implementation is simple, predictable, and requires no additional infrastructure.

5. **Low Risk**: 5-minute TTL is a reasonable "cooldown" period. If this becomes a problem, we can:
   - Make TTL configurable (shorter for testing)
   - Implement cleanup in v2.0 if business need arises

### **Confidence Assessment**

**Overall Confidence**: **90%** ✅

**Justification**:
- ✅ Current behavior is intentional (test comment confirms)
- ✅ No business requirement for immediate cleanup
- ✅ False positive protection is valuable
- ✅ Simple implementation (no additional code)
- ⚠️ 10% risk that immediate cleanup is needed for specific use cases

---

## 🔄 **Reversibility**

**If we need to implement cleanup later**:

1. **Trigger**: User reports that 5-minute TTL is blocking legitimate use cases
2. **Implementation**: Create controller to watch RemediationRequest deletions
3. **Effort**: 4-6 hours
4. **Risk**: Low (additive change, doesn't break existing behavior)

**Monitoring**:
- Track user feedback on CRD deletion behavior
- Monitor if 5-minute TTL causes issues in production
- Re-evaluate in v2.0 if business need arises

---

## 📝 **Action Items**

### **Immediate**
- ✅ Delete Test 3 from Redis integration test implementation plan
- ✅ Document decision in DD-GATEWAY-005
- ✅ Update REDIS_TESTS_IMPLEMENTATION_PLAN.md to reflect decision

### **Future (v2.0)**
- ⏸️ Re-evaluate if user feedback indicates need for immediate cleanup
- ⏸️ Consider making TTL configurable (shorter for testing, longer for production)
- ⏸️ Implement cleanup if business requirement emerges

---

## 🔗 **Related Decisions**

- [DD-GATEWAY-001: Deduplication Strategy](./DD-GATEWAY-001-deduplication-strategy.md) (if exists)
- [DD-GATEWAY-002: Redis Fail-Fast Strategy](./DD-GATEWAY-002-redis-fail-fast-strategy.md) (if exists)

---

## 📚 **References**

- **Test File**: `test/integration/gateway/redis_integration_test.go:238`
- **Implementation**: `pkg/gateway/processing/deduplication.go`
- **Business Requirement**: BR-GATEWAY-008 (Deduplication)

---

## 📊 **Impact Assessment**

### **User Impact**
- **Minimal**: Users rarely delete CRDs manually
- **Expected Behavior**: 5-minute cooldown after CRD deletion is reasonable

### **Developer Impact**
- **Testing**: Developers can use configurable TTL (5 seconds for tests)
- **Integration Tests**: Tests can wait 6 seconds for TTL expiration

### **Operations Impact**
- **Monitoring**: No additional monitoring needed
- **Troubleshooting**: Clear behavior (TTL-based expiration)

---

## 🎯 **Success Criteria**

**How we'll know this decision is correct**:
1. ✅ No user complaints about 5-minute TTL blocking legitimate use cases
2. ✅ Integration tests pass with configurable TTL
3. ✅ False positive protection works as expected
4. ✅ Simple implementation reduces maintenance burden

**How we'll know we need to revisit**:
1. ❌ Users report that 5-minute TTL blocks legitimate use cases
2. ❌ Frequent requests to "reset" alerts by deleting CRDs
3. ❌ Testing workflows blocked by 5-minute delay

---

**Status**: ✅ **DECISION ACCEPTED**
**Next Review**: v2.0 planning (or if user feedback indicates need)
**Owner**: Gateway Service Team

# DD-GATEWAY-005: Redis Cleanup on CRD Deletion

**Status**: ✅ **ACCEPTED** (Option A - No cleanup needed)
**Date**: 2025-10-27
**Deciders**: User, AI Assistant
**Context**: Gateway Integration Tests - Redis cleanup behavior

---

## 🎯 **Decision**

**Do NOT implement automatic Redis fingerprint cleanup when RemediationRequest CRDs are deleted.**

**Current Behavior (KEEP)**:
- Redis deduplication fingerprints expire via TTL (5 minutes default)
- Deleting a RemediationRequest CRD does **not** clean up the Redis fingerprint
- Sending the same alert within 5 minutes will be deduplicated (202 response, no new CRD)

**Rejected Behavior**:
- Automatic cleanup of Redis fingerprints when CRD is deleted
- Immediate recreation of CRD if same alert is sent after CRD deletion

---

## 📋 **Context**

### **Test in Question**
**File**: `test/integration/gateway/redis_integration_test.go:238`
**Test**: `XIt("should clean up Redis state on CRD deletion")`

**Test Expectation** (REJECTED):
1. Send alert → CRD created, Redis fingerprint stored
2. Delete CRD manually
3. Redis fingerprint should be cleaned up immediately
4. Send same alert → New CRD created (not deduplicated)

### **Current Implementation**
**Deduplication Service**:
- Stores fingerprints in Redis with TTL (default: 5 minutes)
- Key format: `dedup:<namespace>:<fingerprint>`
- No cleanup on CRD deletion (TTL-based expiration only)

**CRD Lifecycle**:
- RemediationRequest CRDs may have 24h lifecycle (separate from Redis TTL)
- CRDs can be deleted manually by admins or automatically by controllers
- Redis fingerprints persist for 5 minutes regardless of CRD state

---

## 🤔 **Options Considered**

### **Option A: No Cleanup (Current Behavior)** ✅ **SELECTED**

**Rationale**:
- **Intentional Design**: If an admin deletes a CRD, they likely don't want the same alert to immediately recreate it
- **False Positive Protection**: Deleting a CRD often indicates a false positive or resolved issue
- **Deduplication Window**: 5-minute TTL provides a reasonable "cooldown" period
- **Simplicity**: No additional infrastructure needed (no controller, no K8s API watches)

**Use Cases**:
1. **False Positive**: Admin deletes CRD because alert is incorrect → Same alert shouldn't recreate CRD for 5 minutes
2. **Manual Resolution**: Admin resolves issue and deletes CRD → Same alert shouldn't recreate CRD immediately
3. **Testing**: Developer deletes test CRD → Same alert shouldn't recreate CRD during test cleanup

**Benefits**:
- ✅ Simple implementation (no additional code)
- ✅ Predictable behavior (TTL-based expiration)
- ✅ No K8s API overhead (no CRD watches)
- ✅ Protects against alert storms after CRD deletion

**Drawbacks**:
- ⚠️ 5-minute delay before same alert can create new CRD
- ⚠️ Redis state not immediately synced with K8s state

---

### **Option B: Implement Cleanup** ❌ **REJECTED**

**Rationale**:
- **Immediate Recreation**: If CRD is deleted, same alert can immediately create new CRD
- **State Consistency**: Redis state always synced with K8s state

**Use Cases**:
1. **Reset Alert**: Admin deletes CRD to "reset" alert state → Wants new CRD created immediately
2. **Testing**: Developer wants to test CRD creation multiple times without waiting

**Benefits**:
- ✅ Redis state synced with K8s state
- ✅ Immediate recreation of CRD after deletion

**Drawbacks**:
- ❌ **High Complexity**: Requires controller to watch CRD deletions (4-6 hours implementation)
- ❌ **K8s API Overhead**: Additional watches and API calls
- ❌ **Alert Storm Risk**: Deleting CRD could trigger immediate recreation if alert is still firing
- ❌ **Unclear Business Need**: No documented requirement for this behavior

**Confidence**: 30% (low confidence this is needed)

---

## 📊 **Decision Rationale**

### **Why Option A?**

1. **Intentional Design**: Test comment says *"Current behavior: Redis entries expire via TTL (5 minutes), not immediate cleanup on CRD deletion."* - This suggests intentional design, not a bug.

2. **No Business Requirement**: No documented BR-XXX requirement for immediate Redis cleanup on CRD deletion.

3. **False Positive Protection**: Most common reason to delete a CRD is a false positive or resolved issue. In these cases, you **don't** want the same alert to immediately recreate the CRD.

4. **Simplicity**: Current implementation is simple, predictable, and requires no additional infrastructure.

5. **Low Risk**: 5-minute TTL is a reasonable "cooldown" period. If this becomes a problem, we can:
   - Make TTL configurable (shorter for testing)
   - Implement cleanup in v2.0 if business need arises

### **Confidence Assessment**

**Overall Confidence**: **90%** ✅

**Justification**:
- ✅ Current behavior is intentional (test comment confirms)
- ✅ No business requirement for immediate cleanup
- ✅ False positive protection is valuable
- ✅ Simple implementation (no additional code)
- ⚠️ 10% risk that immediate cleanup is needed for specific use cases

---

## 🔄 **Reversibility**

**If we need to implement cleanup later**:

1. **Trigger**: User reports that 5-minute TTL is blocking legitimate use cases
2. **Implementation**: Create controller to watch RemediationRequest deletions
3. **Effort**: 4-6 hours
4. **Risk**: Low (additive change, doesn't break existing behavior)

**Monitoring**:
- Track user feedback on CRD deletion behavior
- Monitor if 5-minute TTL causes issues in production
- Re-evaluate in v2.0 if business need arises

---

## 📝 **Action Items**

### **Immediate**
- ✅ Delete Test 3 from Redis integration test implementation plan
- ✅ Document decision in DD-GATEWAY-005
- ✅ Update REDIS_TESTS_IMPLEMENTATION_PLAN.md to reflect decision

### **Future (v2.0)**
- ⏸️ Re-evaluate if user feedback indicates need for immediate cleanup
- ⏸️ Consider making TTL configurable (shorter for testing, longer for production)
- ⏸️ Implement cleanup if business requirement emerges

---

## 🔗 **Related Decisions**

- [DD-GATEWAY-001: Deduplication Strategy](./DD-GATEWAY-001-deduplication-strategy.md) (if exists)
- [DD-GATEWAY-002: Redis Fail-Fast Strategy](./DD-GATEWAY-002-redis-fail-fast-strategy.md) (if exists)

---

## 📚 **References**

- **Test File**: `test/integration/gateway/redis_integration_test.go:238`
- **Implementation**: `pkg/gateway/processing/deduplication.go`
- **Business Requirement**: BR-GATEWAY-008 (Deduplication)

---

## 📊 **Impact Assessment**

### **User Impact**
- **Minimal**: Users rarely delete CRDs manually
- **Expected Behavior**: 5-minute cooldown after CRD deletion is reasonable

### **Developer Impact**
- **Testing**: Developers can use configurable TTL (5 seconds for tests)
- **Integration Tests**: Tests can wait 6 seconds for TTL expiration

### **Operations Impact**
- **Monitoring**: No additional monitoring needed
- **Troubleshooting**: Clear behavior (TTL-based expiration)

---

## 🎯 **Success Criteria**

**How we'll know this decision is correct**:
1. ✅ No user complaints about 5-minute TTL blocking legitimate use cases
2. ✅ Integration tests pass with configurable TTL
3. ✅ False positive protection works as expected
4. ✅ Simple implementation reduces maintenance burden

**How we'll know we need to revisit**:
1. ❌ Users report that 5-minute TTL blocks legitimate use cases
2. ❌ Frequent requests to "reset" alerts by deleting CRDs
3. ❌ Testing workflows blocked by 5-minute delay

---

**Status**: ✅ **DECISION ACCEPTED**
**Next Review**: v2.0 planning (or if user feedback indicates need)
**Owner**: Gateway Service Team



**Status**: ✅ **ACCEPTED** (Option A - No cleanup needed)
**Date**: 2025-10-27
**Deciders**: User, AI Assistant
**Context**: Gateway Integration Tests - Redis cleanup behavior

---

## 🎯 **Decision**

**Do NOT implement automatic Redis fingerprint cleanup when RemediationRequest CRDs are deleted.**

**Current Behavior (KEEP)**:
- Redis deduplication fingerprints expire via TTL (5 minutes default)
- Deleting a RemediationRequest CRD does **not** clean up the Redis fingerprint
- Sending the same alert within 5 minutes will be deduplicated (202 response, no new CRD)

**Rejected Behavior**:
- Automatic cleanup of Redis fingerprints when CRD is deleted
- Immediate recreation of CRD if same alert is sent after CRD deletion

---

## 📋 **Context**

### **Test in Question**
**File**: `test/integration/gateway/redis_integration_test.go:238`
**Test**: `XIt("should clean up Redis state on CRD deletion")`

**Test Expectation** (REJECTED):
1. Send alert → CRD created, Redis fingerprint stored
2. Delete CRD manually
3. Redis fingerprint should be cleaned up immediately
4. Send same alert → New CRD created (not deduplicated)

### **Current Implementation**
**Deduplication Service**:
- Stores fingerprints in Redis with TTL (default: 5 minutes)
- Key format: `dedup:<namespace>:<fingerprint>`
- No cleanup on CRD deletion (TTL-based expiration only)

**CRD Lifecycle**:
- RemediationRequest CRDs may have 24h lifecycle (separate from Redis TTL)
- CRDs can be deleted manually by admins or automatically by controllers
- Redis fingerprints persist for 5 minutes regardless of CRD state

---

## 🤔 **Options Considered**

### **Option A: No Cleanup (Current Behavior)** ✅ **SELECTED**

**Rationale**:
- **Intentional Design**: If an admin deletes a CRD, they likely don't want the same alert to immediately recreate it
- **False Positive Protection**: Deleting a CRD often indicates a false positive or resolved issue
- **Deduplication Window**: 5-minute TTL provides a reasonable "cooldown" period
- **Simplicity**: No additional infrastructure needed (no controller, no K8s API watches)

**Use Cases**:
1. **False Positive**: Admin deletes CRD because alert is incorrect → Same alert shouldn't recreate CRD for 5 minutes
2. **Manual Resolution**: Admin resolves issue and deletes CRD → Same alert shouldn't recreate CRD immediately
3. **Testing**: Developer deletes test CRD → Same alert shouldn't recreate CRD during test cleanup

**Benefits**:
- ✅ Simple implementation (no additional code)
- ✅ Predictable behavior (TTL-based expiration)
- ✅ No K8s API overhead (no CRD watches)
- ✅ Protects against alert storms after CRD deletion

**Drawbacks**:
- ⚠️ 5-minute delay before same alert can create new CRD
- ⚠️ Redis state not immediately synced with K8s state

---

### **Option B: Implement Cleanup** ❌ **REJECTED**

**Rationale**:
- **Immediate Recreation**: If CRD is deleted, same alert can immediately create new CRD
- **State Consistency**: Redis state always synced with K8s state

**Use Cases**:
1. **Reset Alert**: Admin deletes CRD to "reset" alert state → Wants new CRD created immediately
2. **Testing**: Developer wants to test CRD creation multiple times without waiting

**Benefits**:
- ✅ Redis state synced with K8s state
- ✅ Immediate recreation of CRD after deletion

**Drawbacks**:
- ❌ **High Complexity**: Requires controller to watch CRD deletions (4-6 hours implementation)
- ❌ **K8s API Overhead**: Additional watches and API calls
- ❌ **Alert Storm Risk**: Deleting CRD could trigger immediate recreation if alert is still firing
- ❌ **Unclear Business Need**: No documented requirement for this behavior

**Confidence**: 30% (low confidence this is needed)

---

## 📊 **Decision Rationale**

### **Why Option A?**

1. **Intentional Design**: Test comment says *"Current behavior: Redis entries expire via TTL (5 minutes), not immediate cleanup on CRD deletion."* - This suggests intentional design, not a bug.

2. **No Business Requirement**: No documented BR-XXX requirement for immediate Redis cleanup on CRD deletion.

3. **False Positive Protection**: Most common reason to delete a CRD is a false positive or resolved issue. In these cases, you **don't** want the same alert to immediately recreate the CRD.

4. **Simplicity**: Current implementation is simple, predictable, and requires no additional infrastructure.

5. **Low Risk**: 5-minute TTL is a reasonable "cooldown" period. If this becomes a problem, we can:
   - Make TTL configurable (shorter for testing)
   - Implement cleanup in v2.0 if business need arises

### **Confidence Assessment**

**Overall Confidence**: **90%** ✅

**Justification**:
- ✅ Current behavior is intentional (test comment confirms)
- ✅ No business requirement for immediate cleanup
- ✅ False positive protection is valuable
- ✅ Simple implementation (no additional code)
- ⚠️ 10% risk that immediate cleanup is needed for specific use cases

---

## 🔄 **Reversibility**

**If we need to implement cleanup later**:

1. **Trigger**: User reports that 5-minute TTL is blocking legitimate use cases
2. **Implementation**: Create controller to watch RemediationRequest deletions
3. **Effort**: 4-6 hours
4. **Risk**: Low (additive change, doesn't break existing behavior)

**Monitoring**:
- Track user feedback on CRD deletion behavior
- Monitor if 5-minute TTL causes issues in production
- Re-evaluate in v2.0 if business need arises

---

## 📝 **Action Items**

### **Immediate**
- ✅ Delete Test 3 from Redis integration test implementation plan
- ✅ Document decision in DD-GATEWAY-005
- ✅ Update REDIS_TESTS_IMPLEMENTATION_PLAN.md to reflect decision

### **Future (v2.0)**
- ⏸️ Re-evaluate if user feedback indicates need for immediate cleanup
- ⏸️ Consider making TTL configurable (shorter for testing, longer for production)
- ⏸️ Implement cleanup if business requirement emerges

---

## 🔗 **Related Decisions**

- [DD-GATEWAY-001: Deduplication Strategy](./DD-GATEWAY-001-deduplication-strategy.md) (if exists)
- [DD-GATEWAY-002: Redis Fail-Fast Strategy](./DD-GATEWAY-002-redis-fail-fast-strategy.md) (if exists)

---

## 📚 **References**

- **Test File**: `test/integration/gateway/redis_integration_test.go:238`
- **Implementation**: `pkg/gateway/processing/deduplication.go`
- **Business Requirement**: BR-GATEWAY-008 (Deduplication)

---

## 📊 **Impact Assessment**

### **User Impact**
- **Minimal**: Users rarely delete CRDs manually
- **Expected Behavior**: 5-minute cooldown after CRD deletion is reasonable

### **Developer Impact**
- **Testing**: Developers can use configurable TTL (5 seconds for tests)
- **Integration Tests**: Tests can wait 6 seconds for TTL expiration

### **Operations Impact**
- **Monitoring**: No additional monitoring needed
- **Troubleshooting**: Clear behavior (TTL-based expiration)

---

## 🎯 **Success Criteria**

**How we'll know this decision is correct**:
1. ✅ No user complaints about 5-minute TTL blocking legitimate use cases
2. ✅ Integration tests pass with configurable TTL
3. ✅ False positive protection works as expected
4. ✅ Simple implementation reduces maintenance burden

**How we'll know we need to revisit**:
1. ❌ Users report that 5-minute TTL blocks legitimate use cases
2. ❌ Frequent requests to "reset" alerts by deleting CRDs
3. ❌ Testing workflows blocked by 5-minute delay

---

**Status**: ✅ **DECISION ACCEPTED**
**Next Review**: v2.0 planning (or if user feedback indicates need)
**Owner**: Gateway Service Team

# DD-GATEWAY-005: Redis Cleanup on CRD Deletion

**Status**: ✅ **ACCEPTED** (Option A - No cleanup needed)
**Date**: 2025-10-27
**Deciders**: User, AI Assistant
**Context**: Gateway Integration Tests - Redis cleanup behavior

---

## 🎯 **Decision**

**Do NOT implement automatic Redis fingerprint cleanup when RemediationRequest CRDs are deleted.**

**Current Behavior (KEEP)**:
- Redis deduplication fingerprints expire via TTL (5 minutes default)
- Deleting a RemediationRequest CRD does **not** clean up the Redis fingerprint
- Sending the same alert within 5 minutes will be deduplicated (202 response, no new CRD)

**Rejected Behavior**:
- Automatic cleanup of Redis fingerprints when CRD is deleted
- Immediate recreation of CRD if same alert is sent after CRD deletion

---

## 📋 **Context**

### **Test in Question**
**File**: `test/integration/gateway/redis_integration_test.go:238`
**Test**: `XIt("should clean up Redis state on CRD deletion")`

**Test Expectation** (REJECTED):
1. Send alert → CRD created, Redis fingerprint stored
2. Delete CRD manually
3. Redis fingerprint should be cleaned up immediately
4. Send same alert → New CRD created (not deduplicated)

### **Current Implementation**
**Deduplication Service**:
- Stores fingerprints in Redis with TTL (default: 5 minutes)
- Key format: `dedup:<namespace>:<fingerprint>`
- No cleanup on CRD deletion (TTL-based expiration only)

**CRD Lifecycle**:
- RemediationRequest CRDs may have 24h lifecycle (separate from Redis TTL)
- CRDs can be deleted manually by admins or automatically by controllers
- Redis fingerprints persist for 5 minutes regardless of CRD state

---

## 🤔 **Options Considered**

### **Option A: No Cleanup (Current Behavior)** ✅ **SELECTED**

**Rationale**:
- **Intentional Design**: If an admin deletes a CRD, they likely don't want the same alert to immediately recreate it
- **False Positive Protection**: Deleting a CRD often indicates a false positive or resolved issue
- **Deduplication Window**: 5-minute TTL provides a reasonable "cooldown" period
- **Simplicity**: No additional infrastructure needed (no controller, no K8s API watches)

**Use Cases**:
1. **False Positive**: Admin deletes CRD because alert is incorrect → Same alert shouldn't recreate CRD for 5 minutes
2. **Manual Resolution**: Admin resolves issue and deletes CRD → Same alert shouldn't recreate CRD immediately
3. **Testing**: Developer deletes test CRD → Same alert shouldn't recreate CRD during test cleanup

**Benefits**:
- ✅ Simple implementation (no additional code)
- ✅ Predictable behavior (TTL-based expiration)
- ✅ No K8s API overhead (no CRD watches)
- ✅ Protects against alert storms after CRD deletion

**Drawbacks**:
- ⚠️ 5-minute delay before same alert can create new CRD
- ⚠️ Redis state not immediately synced with K8s state

---

### **Option B: Implement Cleanup** ❌ **REJECTED**

**Rationale**:
- **Immediate Recreation**: If CRD is deleted, same alert can immediately create new CRD
- **State Consistency**: Redis state always synced with K8s state

**Use Cases**:
1. **Reset Alert**: Admin deletes CRD to "reset" alert state → Wants new CRD created immediately
2. **Testing**: Developer wants to test CRD creation multiple times without waiting

**Benefits**:
- ✅ Redis state synced with K8s state
- ✅ Immediate recreation of CRD after deletion

**Drawbacks**:
- ❌ **High Complexity**: Requires controller to watch CRD deletions (4-6 hours implementation)
- ❌ **K8s API Overhead**: Additional watches and API calls
- ❌ **Alert Storm Risk**: Deleting CRD could trigger immediate recreation if alert is still firing
- ❌ **Unclear Business Need**: No documented requirement for this behavior

**Confidence**: 30% (low confidence this is needed)

---

## 📊 **Decision Rationale**

### **Why Option A?**

1. **Intentional Design**: Test comment says *"Current behavior: Redis entries expire via TTL (5 minutes), not immediate cleanup on CRD deletion."* - This suggests intentional design, not a bug.

2. **No Business Requirement**: No documented BR-XXX requirement for immediate Redis cleanup on CRD deletion.

3. **False Positive Protection**: Most common reason to delete a CRD is a false positive or resolved issue. In these cases, you **don't** want the same alert to immediately recreate the CRD.

4. **Simplicity**: Current implementation is simple, predictable, and requires no additional infrastructure.

5. **Low Risk**: 5-minute TTL is a reasonable "cooldown" period. If this becomes a problem, we can:
   - Make TTL configurable (shorter for testing)
   - Implement cleanup in v2.0 if business need arises

### **Confidence Assessment**

**Overall Confidence**: **90%** ✅

**Justification**:
- ✅ Current behavior is intentional (test comment confirms)
- ✅ No business requirement for immediate cleanup
- ✅ False positive protection is valuable
- ✅ Simple implementation (no additional code)
- ⚠️ 10% risk that immediate cleanup is needed for specific use cases

---

## 🔄 **Reversibility**

**If we need to implement cleanup later**:

1. **Trigger**: User reports that 5-minute TTL is blocking legitimate use cases
2. **Implementation**: Create controller to watch RemediationRequest deletions
3. **Effort**: 4-6 hours
4. **Risk**: Low (additive change, doesn't break existing behavior)

**Monitoring**:
- Track user feedback on CRD deletion behavior
- Monitor if 5-minute TTL causes issues in production
- Re-evaluate in v2.0 if business need arises

---

## 📝 **Action Items**

### **Immediate**
- ✅ Delete Test 3 from Redis integration test implementation plan
- ✅ Document decision in DD-GATEWAY-005
- ✅ Update REDIS_TESTS_IMPLEMENTATION_PLAN.md to reflect decision

### **Future (v2.0)**
- ⏸️ Re-evaluate if user feedback indicates need for immediate cleanup
- ⏸️ Consider making TTL configurable (shorter for testing, longer for production)
- ⏸️ Implement cleanup if business requirement emerges

---

## 🔗 **Related Decisions**

- [DD-GATEWAY-001: Deduplication Strategy](./DD-GATEWAY-001-deduplication-strategy.md) (if exists)
- [DD-GATEWAY-002: Redis Fail-Fast Strategy](./DD-GATEWAY-002-redis-fail-fast-strategy.md) (if exists)

---

## 📚 **References**

- **Test File**: `test/integration/gateway/redis_integration_test.go:238`
- **Implementation**: `pkg/gateway/processing/deduplication.go`
- **Business Requirement**: BR-GATEWAY-008 (Deduplication)

---

## 📊 **Impact Assessment**

### **User Impact**
- **Minimal**: Users rarely delete CRDs manually
- **Expected Behavior**: 5-minute cooldown after CRD deletion is reasonable

### **Developer Impact**
- **Testing**: Developers can use configurable TTL (5 seconds for tests)
- **Integration Tests**: Tests can wait 6 seconds for TTL expiration

### **Operations Impact**
- **Monitoring**: No additional monitoring needed
- **Troubleshooting**: Clear behavior (TTL-based expiration)

---

## 🎯 **Success Criteria**

**How we'll know this decision is correct**:
1. ✅ No user complaints about 5-minute TTL blocking legitimate use cases
2. ✅ Integration tests pass with configurable TTL
3. ✅ False positive protection works as expected
4. ✅ Simple implementation reduces maintenance burden

**How we'll know we need to revisit**:
1. ❌ Users report that 5-minute TTL blocks legitimate use cases
2. ❌ Frequent requests to "reset" alerts by deleting CRDs
3. ❌ Testing workflows blocked by 5-minute delay

---

**Status**: ✅ **DECISION ACCEPTED**
**Next Review**: v2.0 planning (or if user feedback indicates need)
**Owner**: Gateway Service Team




