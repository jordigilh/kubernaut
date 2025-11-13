#!/usr/bin/env python3
"""
PoC: Test sentence-transformers label encoding effectiveness

Purpose: Validate that labels encoded in text affect embedding similarity
as expected for playbook catalog semantic search.

Test Scenarios:
1. Label matching: Same labels should increase similarity
2. Label weight: Labels should influence similarity significantly
3. Partial labels: Query with fewer labels should still match
4. Label vs content: Content similarity vs label similarity trade-off
"""

import numpy as np
from sentence_transformers import SentenceTransformer
from sklearn.metrics.pairwise import cosine_similarity

# Load model (same as production)
print("Loading sentence-transformers/all-MiniLM-L6-v2...")
model = SentenceTransformer('sentence-transformers/all-MiniLM-L6-v2')
print("Model loaded.\n")

# ============================================
# Test 1: Label Matching Effect
# ============================================
print("=" * 60)
print("TEST 1: Label Matching Effect")
print("=" * 60)
print("Hypothesis: Matching labels should increase similarity\n")

playbook_a = """Pod OOM Recovery
Increases memory limits and restarts pod on OOM
environment: production
priority: P0
incident_type: pod-oom-killer
business_category: payment-service"""

playbook_b = """Pod OOM Recovery
Increases memory limits and restarts pod on OOM
environment: staging
priority: P0
incident_type: pod-oom-killer
business_category: payment-service"""

query_prod = """pod keeps crashing with OOM
environment: production
priority: P0
incident_type: pod-oom-killer
business_category: payment-service"""

query_staging = """pod keeps crashing with OOM
environment: staging
priority: P0
incident_type: pod-oom-killer
business_category: payment-service"""

# Generate embeddings
emb_playbook_a = model.encode(playbook_a)
emb_playbook_b = model.encode(playbook_b)
emb_query_prod = model.encode(query_prod)
emb_query_staging = model.encode(query_staging)

# Calculate similarities
sim_query_prod_to_playbook_a = cosine_similarity([emb_query_prod], [emb_playbook_a])[0][0]
sim_query_prod_to_playbook_b = cosine_similarity([emb_query_prod], [emb_playbook_b])[0][0]
sim_query_staging_to_playbook_a = cosine_similarity([emb_query_staging], [emb_playbook_a])[0][0]
sim_query_staging_to_playbook_b = cosine_similarity([emb_query_staging], [emb_playbook_b])[0][0]

print(f"Query (production) → Playbook A (production): {sim_query_prod_to_playbook_a:.4f}")
print(f"Query (production) → Playbook B (staging):    {sim_query_prod_to_playbook_b:.4f}")
print(f"Query (staging)    → Playbook A (production): {sim_query_staging_to_playbook_a:.4f}")
print(f"Query (staging)    → Playbook B (staging):    {sim_query_staging_to_playbook_b:.4f}")

# Expected: Query matches playbook with same environment
test1_pass = (
    sim_query_prod_to_playbook_a > sim_query_prod_to_playbook_b and
    sim_query_staging_to_playbook_b > sim_query_staging_to_playbook_a
)
print(f"\n✅ TEST 1 PASS: {test1_pass}")
print(f"   Expected: production query → production playbook (higher)")
print(f"   Expected: staging query → staging playbook (higher)")
print()

# ============================================
# Test 2: Label Weight vs Content
# ============================================
print("=" * 60)
print("TEST 2: Label Weight vs Content Similarity")
print("=" * 60)
print("Hypothesis: Labels should have significant weight\n")

playbook_oom_prod_p0 = """Pod OOM Recovery
Increases memory limits and restarts pod on OOM
environment: production
priority: P0
incident_type: pod-oom-killer"""

playbook_oom_prod_p1 = """Pod OOM Recovery
Increases memory limits and restarts pod on OOM
environment: production
priority: P1
incident_type: pod-oom-killer"""

playbook_crash_prod_p0 = """Pod Crash Loop Recovery
Restarts pod on crash loop backoff
environment: production
priority: P0
incident_type: crash-loop-backoff"""

query_oom_prod_p0 = """pod keeps crashing with OOM
environment: production
priority: P0
incident_type: pod-oom-killer"""

# Generate embeddings
emb_oom_prod_p0 = model.encode(playbook_oom_prod_p0)
emb_oom_prod_p1 = model.encode(playbook_oom_prod_p1)
emb_crash_prod_p0 = model.encode(playbook_crash_prod_p0)
emb_query_oom = model.encode(query_oom_prod_p0)

# Calculate similarities
sim_oom_p0 = cosine_similarity([emb_query_oom], [emb_oom_prod_p0])[0][0]
sim_oom_p1 = cosine_similarity([emb_query_oom], [emb_oom_prod_p1])[0][0]
sim_crash_p0 = cosine_similarity([emb_query_oom], [emb_crash_prod_p0])[0][0]

print(f"Query (OOM, prod, P0) → Playbook (OOM, prod, P0):   {sim_oom_p0:.4f}")
print(f"Query (OOM, prod, P0) → Playbook (OOM, prod, P1):   {sim_oom_p1:.4f}")
print(f"Query (OOM, prod, P0) → Playbook (Crash, prod, P0): {sim_crash_p0:.4f}")

# Expected: Perfect label match (OOM, prod, P0) should score highest
test2_pass = sim_oom_p0 > sim_oom_p1 and sim_oom_p0 > sim_crash_p0
print(f"\n✅ TEST 2 PASS: {test2_pass}")
print(f"   Expected: Perfect label match scores highest")
print(f"   Content similarity (OOM vs Crash): {sim_crash_p0:.4f}")
print(f"   Label similarity (P0 vs P1): {sim_oom_p1:.4f}")
print()

# ============================================
# Test 3: Partial Label Matching
# ============================================
print("=" * 60)
print("TEST 3: Partial Label Matching")
print("=" * 60)
print("Hypothesis: Query with fewer labels should still match\n")

playbook_full_labels = """Pod OOM Recovery
Increases memory limits and restarts pod on OOM
environment: production
priority: P0
incident_type: pod-oom-killer
business_category: payment-service
mycompany.com/team: platform-engineering
mycompany.com/region: us-east-1"""

query_minimal_labels = """pod keeps crashing with OOM
environment: production
priority: P0
incident_type: pod-oom-killer"""

query_full_labels = """pod keeps crashing with OOM
environment: production
priority: P0
incident_type: pod-oom-killer
business_category: payment-service
mycompany.com/team: platform-engineering
mycompany.com/region: us-east-1"""

# Generate embeddings
emb_playbook_full = model.encode(playbook_full_labels)
emb_query_minimal = model.encode(query_minimal_labels)
emb_query_full = model.encode(query_full_labels)

# Calculate similarities
sim_minimal = cosine_similarity([emb_query_minimal], [emb_playbook_full])[0][0]
sim_full = cosine_similarity([emb_query_full], [emb_playbook_full])[0][0]

print(f"Query (3 labels) → Playbook (7 labels): {sim_minimal:.4f}")
print(f"Query (7 labels) → Playbook (7 labels): {sim_full:.4f}")

# Expected: Full label match should score higher, but minimal should still be good
test3_pass = sim_full > sim_minimal and sim_minimal > 0.7
print(f"\n✅ TEST 3 PASS: {test3_pass}")
print(f"   Expected: Full match > Partial match")
print(f"   Expected: Partial match still > 0.7 (good match)")
print()

# ============================================
# Test 4: Label vs Content Trade-off
# ============================================
print("=" * 60)
print("TEST 4: Label vs Content Trade-off")
print("=" * 60)
print("Hypothesis: Content + labels should outweigh content alone\n")

playbook_perfect_match = """Pod OOM Recovery
Increases memory limits and restarts pod on OOM
environment: production
priority: P0
incident_type: pod-oom-killer"""

playbook_content_only = """Pod OOM Recovery
Increases memory limits and restarts pod on OOM"""

playbook_wrong_labels = """Pod OOM Recovery
Increases memory limits and restarts pod on OOM
environment: staging
priority: P3
incident_type: crash-loop-backoff"""

query_with_labels = """pod keeps crashing with OOM
environment: production
priority: P0
incident_type: pod-oom-killer"""

# Generate embeddings
emb_perfect = model.encode(playbook_perfect_match)
emb_content_only = model.encode(playbook_content_only)
emb_wrong_labels = model.encode(playbook_wrong_labels)
emb_query_labels = model.encode(query_with_labels)

# Calculate similarities
sim_perfect = cosine_similarity([emb_query_labels], [emb_perfect])[0][0]
sim_content = cosine_similarity([emb_query_labels], [emb_content_only])[0][0]
sim_wrong = cosine_similarity([emb_query_labels], [emb_wrong_labels])[0][0]

print(f"Query (with labels) → Playbook (perfect match):  {sim_perfect:.4f}")
print(f"Query (with labels) → Playbook (content only):   {sim_content:.4f}")
print(f"Query (with labels) → Playbook (wrong labels):   {sim_wrong:.4f}")

# Expected: Perfect match > Content only > Wrong labels
test4_pass = sim_perfect > sim_content > sim_wrong
print(f"\n✅ TEST 4 PASS: {test4_pass}")
print(f"   Expected: Perfect match > Content only > Wrong labels")
print()

# ============================================
# Test 5: Custom Label Influence
# ============================================
print("=" * 60)
print("TEST 5: Custom Label Influence")
print("=" * 60)
print("Hypothesis: Custom labels should influence similarity\n")

playbook_team_platform = """Pod OOM Recovery
Increases memory limits and restarts pod on OOM
environment: production
priority: P0
mycompany.com/team: platform-engineering"""

playbook_team_sre = """Pod OOM Recovery
Increases memory limits and restarts pod on OOM
environment: production
priority: P0
mycompany.com/team: sre"""

query_team_platform = """pod keeps crashing with OOM
environment: production
priority: P0
mycompany.com/team: platform-engineering"""

# Generate embeddings
emb_team_platform = model.encode(playbook_team_platform)
emb_team_sre = model.encode(playbook_team_sre)
emb_query_team = model.encode(query_team_platform)

# Calculate similarities
sim_team_match = cosine_similarity([emb_query_team], [emb_team_platform])[0][0]
sim_team_mismatch = cosine_similarity([emb_query_team], [emb_team_sre])[0][0]

print(f"Query (team=platform) → Playbook (team=platform): {sim_team_match:.4f}")
print(f"Query (team=platform) → Playbook (team=sre):      {sim_team_mismatch:.4f}")

# Expected: Matching custom label should score higher
test5_pass = sim_team_match > sim_team_mismatch
print(f"\n✅ TEST 5 PASS: {test5_pass}")
print(f"   Expected: Matching custom label scores higher")
print()

# ============================================
# Summary
# ============================================
print("=" * 60)
print("SUMMARY")
print("=" * 60)
all_tests_pass = test1_pass and test2_pass and test3_pass and test4_pass and test5_pass
print(f"All Tests Pass: {all_tests_pass}")
print(f"  Test 1 (Label Matching):       {test1_pass}")
print(f"  Test 2 (Label Weight):         {test2_pass}")
print(f"  Test 3 (Partial Labels):       {test3_pass}")
print(f"  Test 4 (Label vs Content):     {test4_pass}")
print(f"  Test 5 (Custom Labels):        {test5_pass}")
print()

if all_tests_pass:
    print("✅ CONCLUSION: Label encoding in sentence-transformers is EFFECTIVE")
    print("   Confidence: 95%+")
    print("   Recommendation: Proceed with labels-in-embedding approach")
else:
    print("⚠️  CONCLUSION: Label encoding has LIMITATIONS")
    print("   Recommendation: Consider hybrid approach or alternative models")
print()

# ============================================
# Quantitative Analysis
# ============================================
print("=" * 60)
print("QUANTITATIVE ANALYSIS")
print("=" * 60)
print(f"Label influence (environment mismatch): {abs(sim_query_prod_to_playbook_a - sim_query_prod_to_playbook_b):.4f}")
print(f"Label influence (priority mismatch):    {abs(sim_oom_p0 - sim_oom_p1):.4f}")
print(f"Content influence (OOM vs Crash):       {abs(sim_oom_p0 - sim_crash_p0):.4f}")
print(f"Custom label influence (team):          {abs(sim_team_match - sim_team_mismatch):.4f}")
print()
print("Interpretation:")
print("  > 0.05: Strong influence")
print("  0.02-0.05: Moderate influence")
print("  < 0.02: Weak influence")
print()

