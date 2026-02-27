# NIFTY IT Parent-Child Impact Analysis Design

Date: 2026-02-26
Status: Approved for implementation

## 1. Goal

Implement a permanent pipeline enhancement that supports hierarchical analysis where:

- Parent entities = 10 NIFTY IT companies
- Child entities = AI vendors + macro/technology themes
- Articles are analyzed for parent-child impact relationships
- Output CSV is generated with columns:
  - Index
  - News Source
  - URL
  - Parent entity
  - Child Entity
  - Sentiment
  - Weight
  - Confidence Score
  - Cost
  - Summary (up to 10 words)

Behavior requirements:

1. Parent required, child optional
2. Duplicate records for multiple parent/child combinations in same article
3. Sentiment analyzed per parent-child pair
4. Sentiment field in CSV formatted as: `label (score)`

## 2. Scope

### In Scope

- Config model updates for parent/child roles and entity grouping
- Matching and row generation for parent-child cross-product
- Per-pair sentiment analysis
- CSV schema/output update to requested column format
- Tests for loader, row generation, and exporter behavior

### Out of Scope

- New UI changes
- New API endpoints
- Historical backfill/migration tooling

## 3. Configuration Design

### 3.1 Entity Role Extension

Extend `config.Entity` with a role field:

- `role: parent` for NIFTY IT company entities
- `role: child` for AI/theme entities
- empty role remains valid for legacy compatibility

### 3.2 Entity Group Configuration

Add new file: `configs/entity_groups.yaml`

Purpose: define reusable analysis groups without hardcoding relationships.

Initial group:

- `id: nifty-it-impact`
- `parents`: TCS, INFY, HCLTECH, WIPRO, TECHM, LTIM, MPHASIS, COFORGE, OFSS, PERSISTENT
- `children`: OPENAI, GEMINI, ANTHROPIC, DEEPSEEK, BYTEDANCE, XAI, GEOPOLITICS, GOLD_SILVER_COPPER, SEMICONDUCTOR, NVIDIA, ECONOMICS, ENERGY_ELECTRICITY, AI, DRAM, CHIPS, INDIA_US_RELATIONS

### 3.3 Validation Rules

On config load:

- all group symbols must resolve to enabled entities
- group parents must be `role=parent`
- group children must be `role=child`
- fail fast on unknown symbols and role mismatches

## 4. Data Model Design

### 4.1 FeatureRow Extension

Extend `core.FeatureRow` for parent-child export requirements:

- `NewsSource`
- `URL`
- `ParentEntity`
- `ChildEntity`
- `SentimentDisplay` (formatted as `label (score)`)
- `Weight`
- `ConfidenceScore`
- `Summary`

Internal run metadata remains in `FeatureRow` for provenance and storage consistency.

### 4.2 Cost Allocation

`Cost` column is row-level estimated cost derived from article event cost:

- If an article creates `N` output rows, each row gets `event.EstimatedCostUS / N`

## 5. Matching and Row Generation Design

### 5.1 Parent/Child Selection

For each enriched article:

1. Use existing matching stack (deterministic + LLM disambiguation)
2. Partition matches into:
   - matched parents (role=parent and in group parent set)
   - matched children (role=child and in group child set)

### 5.2 Inclusion Rules

- If no parent matched: skip article
- If parent matched and no child matched: generate one row per parent with `Child Entity = N/A`
- If parent(s) and child(ren) matched: generate Cartesian product rows `(parent, child)`

### 5.3 Duplicate Record Rule

Cross-product generation is the duplication mechanism:

- 2 parents × 3 children => 6 CSV rows from the same article

## 6. Sentiment Design

### 6.1 Per-Pair Sentiment

Run sentiment classification at parent-child pair level.

Input context for each pair:

- article title + summary/body excerpt
- parent entity identity
- child entity identity (or `N/A` for parent-only rows)

Output:

- label (`positive|neutral|negative`)
- score (float)
- confidence (float)

### 6.2 CSV Sentiment Field

Render sentiment as single string:

- `positive (0.62)`
- `neutral (0.05)`
- `negative (-0.48)`

## 7. Export Design

Update exporter CSV headers and mapping to exact required format:

1. Index (1-based row number)
2. News Source
3. URL
4. Parent entity
5. Child Entity
6. Sentiment
7. Weight
8. Confidence Score
9. Cost
10. Summary

Summary is truncated to maximum 10 words.

## 8. Testing Design

### 8.1 Config Tests

- entity group file parses
- unknown symbol in group fails validation
- role mismatch fails validation

### 8.2 Service/Row Generation Tests

- no parent match => no rows
- parent only => rows with `Child Entity = N/A`
- parent + child => cross-product row count correct
- duplicate behavior for multiple parents/children

### 8.3 Exporter Tests

- exact header order
- exact sentiment display format `label (score)`
- row index increments correctly
- summary truncated to 10 words

## 9. Backward Compatibility

- Legacy behavior preserved when group/role config is absent
- Existing ingestion and enrichment flow retained
- New behavior activated by role/group-aware row generation path

## 10. Implementation Outline

1. Add role/group config structures and loader support
2. Add config validation for group references and roles
3. Add child entities and roles in config YAML
4. Extend FeatureRow schema for export fields
5. Implement parent-child row generation logic in engine service
6. Add per-pair sentiment evaluation path
7. Update exporter CSV headers + field mapping
8. Add/adjust unit and integration tests

---

This design is approved by user and ready for implementation planning.