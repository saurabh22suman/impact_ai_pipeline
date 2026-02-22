---
name: pipeline-test-design
description: Generate ingestion and enrichment test cases for AI market intelligence pipeline
argument-hint: "Provide module or flow to design tests for"
allowed-tools: Read,Glob,Grep
---

Design test suites for pipeline components using these dimensions:
- Ingestion correctness (rss parse, crawl fallback, timestamp normalization)
- Deduplication (url hash, content hash, near-duplicate title normalization)
- Entity mapping (NIFTY IT defaults, custom entities, alias disambiguation)
- Relevance gating (factor overlap, novelty score, ambiguity threshold)
- Provider routing (cost-first selection, fallback sequence, outage simulation)
- Export correctness (JSONL, CSV, TOON compatibility)
- Provenance completeness (run_id, config_version, provider/model/prompt_version, token/cost)
- Leakage prevention (point-in-time joins, purged walk-forward with embargo)

Output table format with: case id, scenario, fixture data, expected behavior, assertions.
