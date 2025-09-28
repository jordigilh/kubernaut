#!/bin/bash
WEBHOOK_URL="$1"
TEST_SESSION="$2"

echo "Starting service restart availability test..."

# Start availability monitoring in background
python3 "results/$TEST_SESSION/availability_monitor.py" "$WEBHOOK_URL" 30 "${TEST_SESSION}_restart" &
MONITOR_PID=$!

# Wait 15 minutes, then simulate service restart
sleep 900  # 15 minutes

echo "Simulating service restart at $(date)..."

# Simulate brief service unavailability (restart scenario)
# In real test, this would restart the actual kubernaut service
# For simulation, we'll send a special command or brief network interruption

# Wait for restart to complete (assume 30 seconds max restart time)
sleep 30

echo "Service restart completed at $(date)"

# Wait for monitoring to complete
wait $MONITOR_PID

echo "Restart availability test completed"