#!/bin/bash
# Fix all invalid fingerprints in test files

# hot_reloader_test.go
sed -i '' 's/"hrrv02abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["hr-reload-valid-02"]/g' hot_reloader_test.go
sed -i '' 's/"hrgr01abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["hr-graceful-01"]/g' hot_reloader_test.go
sed -i '' 's/"hrgr02abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["hr-graceful-02"]/g' hot_reloader_test.go
sed -i '' 's/"hrrc01abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["hr-recovery-01"]/g' hot_reloader_test.go
sed -i '' 's/"hrrc02abc123def456abc123def456abc123def456abc123def456abc123d"/ValidTestFingerprints["hr-recovery-02"]/g' hot_reloader_test.go
# Fix concurrent test fingerprints
sed -i '' 's/Fingerprint: "hrcc" \+ string(rune('\''a'\''+idx)) \+ "abc123def456abc123def456abc123def456abc123def456abc123" \+ string(rune('\''0'\''+idx)),/Fingerprint: GenerateConcurrentFingerprint("hr-concurrent", idx),/g' hot_reloader_test.go

echo "Fixed hot_reloader_test.go"
