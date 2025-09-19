#!/bin/bash
WEBHOOK_URL="$1"
TEST_SESSION="$2"

echo "Starting 30-minute sustained load test..."
echo "Rate: 100 alerts/minute"

# Run sustained test in 10-minute phases for monitoring
phases=("0-10min" "10-20min" "20-30min")
results_file="results/$TEST_SESSION/sustained_load_results.json"
echo '{"phases": []}' > "$results_file"

for i in {0..2}; do
    phase=${phases[$i]}
    echo ""
    echo "--- Phase $((i+1))/3: $phase ---"

    # Run 10-minute phase
    python3 results/$TEST_SESSION/detailed_response_time_test.py "$WEBHOOK_URL" "$TEST_SESSION" 100 10

    # Get the latest phase results
    phase_results=$(cat "results/$TEST_SESSION/phase_a1_detailed_results.json")

    # Add phase identifier and append to sustained results
    python3 << PYTHON_EOF
import json

# Load existing results
with open("$results_file", "r") as f:
    sustained_data = json.load(f)

# Load phase results
with open("results/$TEST_SESSION/phase_a1_detailed_results.json", "r") as f:
    phase_data = json.load(f)

# Add phase information
phase_data["phase"] = "$phase"
phase_data["phase_number"] = $((i+1))

# Append to sustained results
sustained_data["phases"].append(phase_data)

# Save updated results
with open("$results_file", "w") as f:
    json.dump(sustained_data, f, indent=2)

# Report phase results
compliance = phase_data["br_pa_003_compliance"]
print(f"Phase $((i+1)) Results:")
print(f"  Success Rate: {phase_data['success_rate']:.2f}%")
print(f"  95th Percentile: {compliance['measured_95th']:.3f}s")
print(f"  Compliance: {'PASS' if compliance['pass'] else 'FAIL'}")
PYTHON_EOF

    # Brief pause between phases for monitoring
    if [ $i -lt 2 ]; then
        echo "Brief pause between phases..."
        sleep 30
    fi
done

# Analyze sustained load results
python3 << 'SUSTAINED_ANALYSIS_EOF'
import json
import statistics

with open("results/" + "$TEST_SESSION" + "/sustained_load_results.json", "r") as f:
    data = json.load(f)

print("\n=== Sustained Load Analysis ===")
print("Phase | Success Rate | 95th Percentile | Compliance")
print("------|--------------|-----------------|----------")

all_95th_percentiles = []
all_success_rates = []
passing_phases = 0

for phase in data["phases"]:
    success_rate = phase["success_rate"]
    percentile_95 = phase["br_pa_003_compliance"]["measured_95th"]
    compliance = "PASS" if phase["br_pa_003_compliance"]["pass"] else "FAIL"

    print(f"{phase['phase']:>5} | {success_rate:>10.1f}% | {percentile_95:>13.3f}s | {compliance}")

    all_95th_percentiles.append(percentile_95)
    all_success_rates.append(success_rate)
    if compliance == "PASS":
        passing_phases += 1

# Overall sustained performance
print(f"\n=== Overall Sustained Performance ===")
print(f"Phases Passed: {passing_phases}/3")
print(f"Average 95th Percentile: {statistics.mean(all_95th_percentiles):.3f}s")
print(f"Max 95th Percentile: {max(all_95th_percentiles):.3f}s")
print(f"Average Success Rate: {statistics.mean(all_success_rates):.2f}%")

# Performance consistency check
percentile_variance = statistics.variance(all_95th_percentiles) if len(all_95th_percentiles) > 1 else 0
print(f"Performance Consistency (variance): {percentile_variance:.6f}")

# Final validation
sustained_pass = passing_phases == 3 and max(all_95th_percentiles) < 5.0
print(f"\n=== Phase A2 Result ===")
print(f"Sustained Load Validation: {'✅ PASS' if sustained_pass else '❌ FAIL'}")

if sustained_pass:
    print("System demonstrates consistent performance under sustained load")
else:
    print("System shows performance degradation or inconsistency under sustained load")

SUSTAINED_ANALYSIS_EOF