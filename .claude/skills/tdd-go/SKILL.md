---
name: tdd-go
description: Enforce red green refactor workflow for Go tasks
argument-hint: "Describe the Go feature or bug to implement"
allowed-tools: Read,Write,Edit,Glob,Grep,Bash
---

Follow strict red-green-refactor discipline for Go code:

1) Red
- Write or update failing tests first.
- Confirm failure with targeted `go test`.

2) Green
- Implement minimal production code to satisfy tests.
- Avoid adding behavior not covered by tests.

3) Refactor
- Improve clarity while preserving behavior.
- Keep tests green at every step.

4) Validate
- Run `go test ./...`
- Run `go test ./... -race`
- Run focused fuzz targets where parser/mapper logic changed.

Return a concise checklist of evidence for each phase.
