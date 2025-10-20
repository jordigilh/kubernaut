# Kubernetes Compact Cluster Issue and Resolution

## Issue Summary

During Kubernetes 4.18 deployment using KCLI, the cluster was deployed as a **compact cluster** (3 control+worker nodes) instead of the intended **3+3 topology** (3 control + 3 dedicated worker nodes), causing severe resource constraints and preventing additional operators from scheduling.

## Root Cause Analysis

### What Happened
1. **Expected**: 6 nodes total (3 control + 3 workers)
2. **Actual**: 3 nodes with combined `control-plane,master,worker` roles
3. **Result**: All services running on 3 nodes caused 99-100% CPU allocation
4. **Consequence**: Local Storage Operator pods couldn't schedule due to resource constraints

### Technical Root Cause

The issue was in the KCLI configuration that caused Kubernetes to install with `platform: none` instead of proper platform configuration:

```yaml
# Problem Configuration (install-config.yaml)
platform:
  none: {}                    # ❌ Platform-agnostic mode
compute:
- name: worker
  replicas: 3                 # ❌ Workers defined but not managed
```

**Why this happened:**
- KCLI configuration included `baremetal_hosts` section for virtualized deployment
- This triggered bare metal provisioning mode in virtualized environment
- Kubernetes fell back to platform-agnostic mode (`platform: none`)
- Without machine API management, control planes became schedulable workers

### Resource Impact

Each control+worker node showed severe resource pressure:
```
Node 1: CPU requests 3500m/3500m (100% allocated)
Node 2: CPU requests 3498m/3500m (99% allocated)
Node 3: CPU requests 3499m/3500m (99% allocated)
```

Services running on each node:
- **Control Plane**: etcd, API server, scheduler, controller-manager
- **Network**: OVN pods, CoreDNS, ingress controllers
- **Storage**: Full Ceph cluster (Mon, MGR, OSD pods)
- **Monitoring**: Prometheus, Grafana, AlertManager
- **Registry**: Internal container registry

## Immediate Resolution

### Step 1: Scale the Cluster
```bash
# Add missing worker nodes
kcli scale kube openshift ocp418-baremetal --workers 3
```

### Step 2: Remove Problematic Local Storage
```bash
# Remove failing LocalVolume configuration
oc delete localvolume localstorage-disks -n openshift-local-storage
```

### Step 3: Validation
```bash
# Monitor worker nodes joining
oc get nodes

# Check resource allocation improves
oc describe nodes | grep "Allocated resources" -A 3
```

## Permanent Fix in KCLI Configuration

### Updated Configuration: `kcli-baremetal-params-root.yml`

```yaml
# ❌ PROBLEM: Original configuration
baremetal_hosts:
  - name: master-0
    mac: "aa:bb:cc:dd:ee:f0"
    ipmi_address: "192.168.122.10"
    # ... (causes platform: none)

# ✅ SOLUTION: Updated configuration
# CRITICAL: Disable bare metal provisioning for virtualized deployment
provisioning_enable: false
baremetal: false

# VIRTUALIZED DEPLOYMENT - No bare metal hosts needed
# KCLI will create VMs automatically based on ctlplane/workers counts
# Remove baremetal_hosts section to prevent platform: none configuration
```

### Key Changes Made

1. **Set `baremetal: false`** - Prevents platform-agnostic mode
2. **Removed `baremetal_hosts` section** - Allows proper VM-based deployment
3. **Disabled `local_storage`** - VMs don't have additional block devices
4. **Updated documentation** - Clear separation between VM and physical deployments

### Separate Physical Hardware Configuration

Created `kcli-baremetal-params-physical.yml` for actual bare metal deployments:
```yaml
# For ACTUAL physical hardware with IPMI/Redfish
baremetal: true
provisioning_enable: true
baremetal_hosts:
  # Actual physical server inventory
```

## Prevention Guidelines

### For Virtualized Deployments
✅ **Use**: `kcli-baremetal-params-root.yml` (updated)
- Set `baremetal: false`
- No `baremetal_hosts` section
- Workers created as VMs automatically

### For Physical Bare Metal
✅ **Use**: `kcli-baremetal-params-physical.yml`
- Set `baremetal: true`
- Include `baremetal_hosts` with IPMI details
- Enable provisioning network

## Validation Steps

### Verify Proper Deployment
```bash
# Check node topology (should show dedicated workers after fix)
oc get nodes --show-labels

# Verify platform configuration
oc get infrastructure cluster -o yaml | grep platform

# Check machine sets exist (should show worker machine sets)
oc get machinesets -n openshift-machine-api

# Verify resource distribution
oc describe nodes | grep -E "Name:|Allocated resources:" -A 3
```

### Expected Results After Fix
- ✅ 6 total nodes (3 control + 3 worker)
- ✅ Control plane nodes: `control-plane,master` roles only
- ✅ Worker nodes: `worker` role only
- ✅ Balanced resource allocation across nodes
- ✅ Storage and other operators can schedule properly

## Related Files Modified

1. **`kcli-baremetal-params-root.yml`** - Fixed virtualized deployment config
2. **`kcli-baremetal-params-physical.yml`** - New physical hardware config
3. **`COMPACT_CLUSTER_ISSUE_AND_FIX.md`** - This documentation

## Monitoring and Alerting

### Key Metrics to Watch
```bash
# Node resource utilization
oc adm top nodes

# Pod scheduling issues
oc get pods --all-namespaces --field-selector=status.phase=Pending

# Machine API health
oc get machines -n openshift-machine-api
oc get machinesets -n openshift-machine-api
```

### Warning Signs of Compact Cluster
- All nodes have `control-plane,master,worker` roles
- No machine sets in `openshift-machine-api` namespace
- High resource pressure on control plane nodes
- Pods failing to schedule due to resource constraints
- `platform: none` in cluster infrastructure configuration

## Lessons Learned

1. **Configuration Context Matters**: Same config file doesn't work for both VM and physical deployments
2. **Resource Planning**: Compact clusters require careful resource planning
3. **Platform Detection**: KCLI platform detection can be confused by mixed configurations
4. **Early Validation**: Check node topology immediately after deployment
5. **Separate Configs**: Maintain distinct configurations for different deployment types

This issue cost significant troubleshooting time but provides valuable insights for future Kubernetes deployments using KCLI.
