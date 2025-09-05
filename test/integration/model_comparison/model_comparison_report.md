# Model Comparison Report (Ollama Demo)

## Summary

### granite3.3:2b
- **Overall Rating**: Poor
- **Action Accuracy**: 0.00%
- **Success Rate**: 100.00%
- **Avg Response Time**: 11.02925ms
- **P95 Response Time**: 11.02925ms

## Demo Mode Notes

This was run in **demo mode** using ollama with model switching.

**Limitations:**
- Single ollama instance (model switching overhead)
- Limited test scenarios (5 vs full 15)
- Single run per scenario (vs 3 runs for consistency)

**For Production Comparison:**
- Install ramallama: `cargo install ramallama`
- Run: `make model-comparison-full`

