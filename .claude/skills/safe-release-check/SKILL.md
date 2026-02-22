---
name: safe-release-check
description: Run pre-release reliability and security checklist for Go services
argument-hint: "Optional release tag or branch context"
allowed-tools: Read,Glob,Grep,Bash
---

Pre-release checklist:
- all tests pass (`go test ./...` and race)
- no critical lint findings
- no known high severity vulnerabilities (govulncheck)
- configs validated and versioned
- docker compose services boot with healthy dependencies
- fallback routing validated with one provider disabled
- output reproducibility validated via provenance fields

Return PASS/BLOCKED per item and explicit blockers.
