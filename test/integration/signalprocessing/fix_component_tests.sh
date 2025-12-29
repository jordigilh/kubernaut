#!/bin/bash
# Fix component_integration_test.go fingerprints

sed -i '' 's/"pod001abc123def456abc123def456abc123def456abc123def456abc123de"/ValidTestFingerprints["enrich-pod"]/g' component_integration_test.go
sed -i '' 's/"dep001abc123def456abc123def456abc123def456abc123def456abc123de"/ValidTestFingerprints["enrich-deploy"]/g' component_integration_test.go
sed -i '' 's/"sts001abc123def456abc123def456abc123def456abc123def456abc123de"/ValidTestFingerprints["enrich-sts"]/g' component_integration_test.go
sed -i '' 's/"svc001abc123def456abc123def456abc123def456abc123def456abc123de"/ValidTestFingerprints["enrich-svc"]/g' component_integration_test.go
sed -i '' 's/"nsc001abc123def456abc123def456abc123def456abc123def456abc123de"/ValidTestFingerprints["enrich-ns"]/g' component_integration_test.go
sed -i '' 's/"deg001abc123def456abc123def456abc123def456abc123def456abc123de"/ValidTestFingerprints["enrich-degraded"]/g' component_integration_test.go
sed -i '' 's/"env001abc123def456abc123def456abc123def456abc123def456abc123de"/ValidTestFingerprints["env-configmap"]/g' component_integration_test.go
sed -i '' 's/"lbl001abc123def456abc123def456abc123def456abc123def456abc123de"/ValidTestFingerprints["env-label"]/g' component_integration_test.go
sed -i '' 's/"pri001abc123def456abc123def456abc123def456abc123def456abc123de"/ValidTestFingerprints["priority-rego"]/g' component_integration_test.go
sed -i '' 's/"fal001abc123def456abc123def456abc123def456abc123def456abc123de"/ValidTestFingerprints["priority-fallback"]/g' component_integration_test.go
sed -i '' 's/"pcm001abc123def456abc123def456abc123def456abc123def456abc123de"/ValidTestFingerprints["priority-cm"]/g' component_integration_test.go
sed -i '' 's/"bus001abc123def456abc123def456abc123def456abc123def456abc123de"/ValidTestFingerprints["business-label"]/g' component_integration_test.go
sed -i '' 's/"bup001abc123def456abc123def456abc123def456abc123def456abc123de"/ValidTestFingerprints["business-pattern"]/g' component_integration_test.go
sed -i '' 's/"own001abc123def456abc123def456abc123def456abc123def456abc123de"/ValidTestFingerprints["ownerchain"]/g' component_integration_test.go
sed -i '' 's/"pdb001abc123def456abc123def456abc123def456abc123def456abc123de"/ValidTestFingerprints["detect-pdb"]/g' component_integration_test.go
sed -i '' 's/"hpa001abc123def456abc123def456abc123def456abc123def456abc123de"/ValidTestFingerprints["detect-hpa"]/g' component_integration_test.go
sed -i '' 's/"net001abc123def456abc123def456abc123def456abc123def456abc123de"/ValidTestFingerprints["detect-netpol"]/g' component_integration_test.go

echo "Fixed component_integration_test.go"
