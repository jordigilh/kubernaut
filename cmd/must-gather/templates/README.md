# Kubernaut Must-Gather - RBAC Templates

These RBAC manifests are **required** for must-gather to function. The must-gather container needs read-only access to collect diagnostic data.

## Quick Start

### Deploy RBAC resources

```bash
# Using kubectl
kubectl apply -f templates/

# Using kustomize
kubectl apply -k templates/
```

### Verify deployment

```bash
# Check ServiceAccount
kubectl get serviceaccount kubernaut-must-gather -n default

# Check ClusterRole
kubectl get clusterrole kubernaut-must-gather

# Check ClusterRoleBinding
kubectl get clusterrolebinding kubernaut-must-gather

# Test permissions
kubectl auth can-i get pods --all-namespaces \
  --as=system:serviceaccount:default:kubernaut-must-gather
```

## Usage with Must-Gather

### OpenShift

```bash
oc adm must-gather \
  --image=quay.io/kubernaut/must-gather:latest \
  -- /usr/bin/gather
```

OpenShift automatically creates a ServiceAccount with sufficient privileges.

### Kubernetes

```bash
# Ensure RBAC is deployed first!
kubectl apply -f templates/

# Run must-gather pod
kubectl run kubernaut-must-gather \
  --image=quay.io/kubernaut/must-gather:latest \
  --rm --attach --restart=Never \
  --serviceaccount=kubernaut-must-gather \
  -- /usr/bin/gather

# Or using debug node (requires node access)
kubectl debug node/<node-name> \
  --image=quay.io/kubernaut/must-gather:latest \
  --image-pull-policy=Always \
  -- /usr/bin/gather
```

## Security Considerations

### Read-Only Access

The `kubernaut-must-gather` ClusterRole grants **read-only** access:
- ✅ Can read pod logs
- ✅ Can list CRDs and resources
- ✅ Can get ConfigMaps and Secrets
- ❌ Cannot create, update, or delete resources
- ❌ Cannot execute commands in pods
- ❌ Cannot modify cluster state

### Secrets Access

Must-gather can read Secret **metadata** but automatically sanitizes Secret **values** in the output. See [BR-PLATFORM-001.9](../../../docs/requirements/BR-PLATFORM-001-must-gather-diagnostic-collection.md) for sanitization details.

### Cleanup

```bash
# Remove RBAC resources when done
kubectl delete -f templates/

# Or with kustomize
kubectl delete -k templates/
```

## Namespace Customization

By default, the ServiceAccount is created in the `default` namespace. To use a different namespace:

```bash
# Edit serviceaccount.yaml and clusterrolebinding.yaml
sed -i 's/namespace: default/namespace: kubernaut-system/g' templates/*.yaml

# Apply
kubectl apply -f templates/
```

## OpenShift SecurityContextConstraints

On OpenShift, must-gather may require additional SCC privileges:

```bash
# Grant privileged SCC (if needed for node access)
oc adm policy add-scc-to-user privileged \
  system:serviceaccount:default:kubernaut-must-gather
```

This is typically **not required** for standard must-gather operations.

## Troubleshooting

### Permission Denied Errors

If must-gather fails with permission errors:

```bash
# Check if RBAC resources exist
kubectl get clusterrole kubernaut-must-gather
kubectl get clusterrolebinding kubernaut-must-gather
kubectl get serviceaccount kubernaut-must-gather -n default

# Verify permissions
kubectl auth can-i get pods --all-namespaces \
  --as=system:serviceaccount:default:kubernaut-must-gather

# Check for RBAC errors in events
kubectl get events --all-namespaces | grep -i "forbidden\|denied"
```

### ServiceAccount Not Found

```bash
# Ensure ServiceAccount is in the correct namespace
kubectl get serviceaccount kubernaut-must-gather -n default

# If missing, recreate
kubectl apply -f templates/serviceaccount.yaml
```

## Reference

- [BR-PLATFORM-001](../../../docs/requirements/BR-PLATFORM-001-must-gather-diagnostic-collection.md) - Business requirements
- [Kubernetes RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/) - Official documentation
- [OpenShift must-gather](https://docs.openshift.com/container-platform/latest/support/gathering-cluster-data.html) - Pattern reference

