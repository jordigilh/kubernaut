#!/bin/bash
# Gateway E2E Infrastructure Validation Script
# Validates: Kind cluster, Redis Sentinel HA, AlertManager

set -e

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Gateway E2E Infrastructure Validation"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Test 1: Kind Cluster Nodes
echo "✓ Test 1: Validating Kind cluster nodes..."
NODE_COUNT=$(kubectl get nodes --no-headers | wc -l | tr -d ' ')
if [ "$NODE_COUNT" -ne "4" ]; then
    echo "❌ FAIL: Expected 4 nodes, found $NODE_COUNT"
    exit 1
fi
echo "✅ PASS: 4 nodes ready (1 control-plane + 3 workers)"
echo ""

# Test 2: Redis Master
echo "✓ Test 2: Validating Redis master..."
REDIS_MASTER_STATUS=$(kubectl get pod redis-master-0 -n kubernaut-system -o jsonpath='{.status.phase}')
if [ "$REDIS_MASTER_STATUS" != "Running" ]; then
    echo "❌ FAIL: Redis master not running (status: $REDIS_MASTER_STATUS)"
    exit 1
fi
echo "✅ PASS: Redis master running"
echo ""

# Test 3: Redis Replicas
echo "✓ Test 3: Validating Redis replicas..."
REDIS_REPLICA_COUNT=$(kubectl get pods -n kubernaut-system -l role=replica --no-headers | grep Running | wc -l | tr -d ' ')
if [ "$REDIS_REPLICA_COUNT" -ne "2" ]; then
    echo "❌ FAIL: Expected 2 replicas, found $REDIS_REPLICA_COUNT running"
    exit 1
fi
echo "✅ PASS: 2 Redis replicas running"
echo ""

# Test 4: Redis Sentinels
echo "✓ Test 4: Validating Redis Sentinels..."
SENTINEL_COUNT=$(kubectl get pods -n kubernaut-system -l app=redis-sentinel --no-headers | grep Running | wc -l | tr -d ' ')
if [ "$SENTINEL_COUNT" -ne "3" ]; then
    echo "❌ FAIL: Expected 3 Sentinels, found $SENTINEL_COUNT running"
    exit 1
fi
echo "✅ PASS: 3 Redis Sentinels running"
echo ""

# Test 5: Sentinel Master Monitoring
echo "✓ Test 5: Validating Sentinel master monitoring..."
SENTINEL_MASTER=$(kubectl exec redis-sentinel-0 -n kubernaut-system -- redis-cli -p 26379 sentinel get-master-addr-by-name mymaster 2>/dev/null | head -1)
if [ -z "$SENTINEL_MASTER" ]; then
    echo "❌ FAIL: Sentinel not monitoring master"
    exit 1
fi
echo "✅ PASS: Sentinel monitoring master at $SENTINEL_MASTER"
echo ""

# Test 6: Sentinel Quorum
echo "✓ Test 6: Validating Sentinel quorum..."
SENTINEL_QUORUM=$(kubectl exec redis-sentinel-0 -n kubernaut-system -- redis-cli -p 26379 sentinel masters 2>/dev/null | grep -A 1 "^quorum$" | tail -1)
if [ "$SENTINEL_QUORUM" != "2" ]; then
    echo "❌ FAIL: Expected quorum 2, found $SENTINEL_QUORUM"
    exit 1
fi
echo "✅ PASS: Sentinel quorum configured (2/3)"
echo ""

# Test 7: Redis Connectivity
echo "✓ Test 7: Validating Redis connectivity..."
REDIS_PING=$(kubectl exec redis-master-0 -n kubernaut-system -- redis-cli ping 2>/dev/null)
if [ "$REDIS_PING" != "PONG" ]; then
    echo "❌ FAIL: Redis not responding to PING"
    exit 1
fi
echo "✅ PASS: Redis responding to PING"
echo ""

# Test 8: Redis Replication
echo "✓ Test 8: Validating Redis replication..."
CONNECTED_SLAVES=$(kubectl exec redis-master-0 -n kubernaut-system -- redis-cli info replication 2>/dev/null | grep "connected_slaves:" | cut -d: -f2 | tr -d '\r')
if [ "$CONNECTED_SLAVES" != "2" ]; then
    echo "❌ FAIL: Expected 2 connected slaves, found $CONNECTED_SLAVES"
    exit 1
fi
echo "✅ PASS: 2 replicas connected to master"
echo ""

# Test 9: AlertManager
echo "✓ Test 9: Validating AlertManager..."
ALERTMANAGER_STATUS=$(kubectl get pods -n kubernaut-system -l app=alertmanager -o jsonpath='{.items[0].status.phase}')
if [ "$ALERTMANAGER_STATUS" != "Running" ]; then
    echo "❌ FAIL: AlertManager not running (status: $ALERTMANAGER_STATUS)"
    exit 1
fi
echo "✅ PASS: AlertManager running"
echo ""

# Test 10: AlertManager Health
echo "✓ Test 10: Validating AlertManager health..."
ALERTMANAGER_POD=$(kubectl get pods -n kubernaut-system -l app=alertmanager -o jsonpath='{.items[0].metadata.name}')
kubectl exec $ALERTMANAGER_POD -n kubernaut-system -- wget -q -O- http://localhost:9093/-/healthy > /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "❌ FAIL: AlertManager health check failed"
    exit 1
fi
echo "✅ PASS: AlertManager healthy"
echo ""

# Summary
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ ALL VALIDATION TESTS PASSED (10/10)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "Infrastructure Summary:"
echo "  • Kind Cluster: 4 nodes (1 control-plane + 3 workers)"
echo "  • Redis Master: 1 instance"
echo "  • Redis Replicas: 2 instances"
echo "  • Redis Sentinels: 3 instances (quorum: 2)"
echo "  • AlertManager: 1 instance"
echo ""
echo "✅ Infrastructure ready for E2E testing"

