#!/bin/bash
set +e
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut-v1.5
sleep 30
while true; do
  echo "=== $(date) ==="
  OUTPUT=$(gh pr checks 1328 2>&1 || true)
  FAILS=$(echo "$OUTPUT" | grep -c 'fail' || true)
  PASSES=$(echo "$OUTPUT" | grep -c 'pass' || true)
  PENDING=$(echo "$OUTPUT" | grep -c 'pending' || true)
  echo "pass=$PASSES fail=$FAILS pending=$PENDING"
  if [ "$PENDING" -eq 0 ] && [ "$FAILS" -eq 0 ] && [ "$PASSES" -gt 0 ]; then
    echo "ALL_CHECKS_PASSED"
    exit 0
  fi
  if [ "$FAILS" -gt 0 ]; then
    echo "$OUTPUT" | grep 'fail'
  fi
  if [ "$PENDING" -eq 0 ] && [ "$FAILS" -gt 0 ]; then
    echo "ALL_DONE_WITH_FAILURES"
    exit 1
  fi
  sleep 60
done
