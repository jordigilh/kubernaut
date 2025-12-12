#!/bin/bash
# Fix rego_integration_test.go fingerprints

sed -i '' 's/"renv01abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["rego-env-01"]/g' rego_integration_test.go
sed -i '' 's/"rpri01abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["rego-pri-01"]/g' rego_integration_test.go
sed -i '' 's/"rlbl01abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["rego-lbl-01"]/g' rego_integration_test.go
sed -i '' 's/"reve01abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["rego-eve-01"]/g' rego_integration_test.go
sed -i '' 's/"revp01abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["rego-evp-01"]/g' rego_integration_test.go
sed -i '' 's/"revl01abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["rego-evl-01"]/g' rego_integration_test.go
sed -i '' 's/"rsec01abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["rego-sec-01"]/g' rego_integration_test.go
sed -i '' 's/"rfin01abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["rego-fin-01"]/g' rego_integration_test.go
sed -i '' 's/"rfms01abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["rego-fms-01"]/g' rego_integration_test.go
sed -i '' 's/"rtim01abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["rego-tim-01"]/g' rego_integration_test.go
sed -i '' 's/"rvlk01abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["rego-vlk-01"]/g' rego_integration_test.go
sed -i '' 's/"rvlv01abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["rego-vlv-01"]/g' rego_integration_test.go
sed -i '' 's/"rvmk01abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["rego-vmk-01"]/g' rego_integration_test.go
# Fix concurrent test fingerprints
sed -i '' 's/Fingerprint: "rcon" \+ string(rune('\''a'\''+idx)) \+ "abc123def456abc123def456abc123def456abc123def456abc123" \+ string(rune('\''0'\''+idx)),/Fingerprint: GenerateConcurrentFingerprint("rego-concurrent", idx),/g' rego_integration_test.go

echo "Fixed rego_integration_test.go"
