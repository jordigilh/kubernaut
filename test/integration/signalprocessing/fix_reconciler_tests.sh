#!/bin/bash
# Fix reconciler_integration_test.go fingerprints

sed -i '' 's/"abc123def456abc123def456abc123def456abc123def456abc123def456abc1"/ValidTestFingerprints["reconciler-01"]/g' reconciler_integration_test.go
sed -i '' 's/"bbb123def456abc123def456abc123def456abc123def456abc123def456abc2"/ValidTestFingerprints["reconciler-02"]/g' reconciler_integration_test.go
sed -i '' 's/"ccc123def456abc123def456abc123def456abc123def456abc123def456abc3"/ValidTestFingerprints["reconciler-03"]/g' reconciler_integration_test.go
sed -i '' 's/"ddd123def456abc123def456abc123def456abc123def456abc123def456abc4"/ValidTestFingerprints["reconciler-04"]/g' reconciler_integration_test.go
sed -i '' 's/"eee123def456abc123def456abc123def456abc123def456abc123def456abc5"/ValidTestFingerprints["reconciler-05"]/g' reconciler_integration_test.go
sed -i '' 's/"fff123def456abc123def456abc123def456abc123def456abc123def456abc6"/ValidTestFingerprints["reconciler-06"]/g' reconciler_integration_test.go
sed -i '' 's/"ggg123def456abc123def456abc123def456abc123def456abc123def456abc7"/ValidTestFingerprints["reconciler-07"]/g' reconciler_integration_test.go
sed -i '' 's/"hhh123def456abc123def456abc123def456abc123def456abc123def456abc8"/ValidTestFingerprints["reconciler-08"]/g' reconciler_integration_test.go
sed -i '' 's/"iii123def456abc123def456abc123def456abc123def456abc123def456abc9"/ValidTestFingerprints["reconciler-09"]/g' reconciler_integration_test.go
sed -i '' 's/"jjj123def456abc123def456abc123def456abc123def456abc123def456ab10"/ValidTestFingerprints["reconciler-10"]/g' reconciler_integration_test.go
sed -i '' 's/"ec01def456abc123def456abc123def456abc123def456abc123def456abc01"/ValidTestFingerprints["edge-case-01"]/g' reconciler_integration_test.go
sed -i '' 's/"ec02def456abc123def456abc123def456abc123def456abc123def456abc02"/ValidTestFingerprints["edge-case-02"]/g' reconciler_integration_test.go
sed -i '' 's/"ec04def456abc123def456abc123def456abc123def456abc123def456abc04"/ValidTestFingerprints["edge-case-04"]/g' reconciler_integration_test.go
sed -i '' 's/"ec05def456abc123def456abc123def456abc123def456abc123def456abc05"/ValidTestFingerprints["edge-case-05"]/g' reconciler_integration_test.go
sed -i '' 's/"ec06def456abc123def456abc123def456abc123def456abc123def456abc06"/ValidTestFingerprints["edge-case-06"]/g' reconciler_integration_test.go
sed -i '' 's/"ec07def456abc123def456abc123def456abc123def456abc123def456abc07"/ValidTestFingerprints["edge-case-07"]/g' reconciler_integration_test.go
sed -i '' 's/"ec08def456abc123def456abc123def456abc123def456abc123def456abc08"/ValidTestFingerprints["edge-case-08"]/g' reconciler_integration_test.go
sed -i '' 's/"er02def456abc123def456abc123def456abc123def456abc123def456ab002"/ValidTestFingerprints["error-02"]/g' reconciler_integration_test.go
sed -i '' 's/"er04def456abc123def456abc123def456abc123def456abc123def456ab004"/ValidTestFingerprints["error-04"]/g' reconciler_integration_test.go
sed -i '' 's/"er06def456abc123def456abc123def456abc123def456abc123def456ab006"/ValidTestFingerprints["error-06"]/g' reconciler_integration_test.go
sed -i '' 's/"a001def456abc123def456abc123def456abc123def456abc123def456abc001"/ValidTestFingerprints["audit-001"]/g' reconciler_integration_test.go
sed -i '' 's/"a002def456abc123def456abc123def456abc123def456abc123def456abc002"/ValidTestFingerprints["audit-002"]/g' reconciler_integration_test.go
sed -i '' 's/"a003def456abc123def456abc123def456abc123def456abc123def456abc003"/ValidTestFingerprints["audit-003"]/g' reconciler_integration_test.go
sed -i '' 's/"a004def456abc123def456abc123def456abc123def456abc123def456abc004"/ValidTestFingerprints["audit-004"]/g' reconciler_integration_test.go
# Fix concurrent test fingerprints
sed -i '' 's/Fingerprint: "conc" \+ string(rune('\''a'\''+idx)) \+ "def456abc123def456abc123def456abc123def456abc123def456abc0" \+ string(rune('\''0'\''+idx)),/Fingerprint: GenerateConcurrentFingerprint("reconciler-concurrent", idx),/g' reconciler_integration_test.go

echo "Fixed reconciler_integration_test.go"
