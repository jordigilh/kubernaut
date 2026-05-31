#!/bin/bash
set +e
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut-v1.5
while true; do
  result=$(gh pr checks 1328 2>&1; true)
  failed=$(echo "$result" | grep -c "fail")
  passed=$(echo "$result" | grep -c "pass")
  pending=$(echo "$result" | grep -c "pending")
  total=$((failed + passed + pending))
  echo "$(date '+%H:%M:%S') — passed=${passed} failed=${failed} pending=${pending} total=${total}"
  if [ "${total}" -lt 5 ]; then
    sleep 60
    continue
  fi
  if [ "${failed}" -gt 0 ]; then
    echo "FAILURE_DETECTED"
    echo "$result" | grep "fail"
  fi
  if [ "${pending}" -eq 0 ]; then
    echo "ALL_CHECKS_COMPLETE"
    break
  fi
  sleep 120
done
