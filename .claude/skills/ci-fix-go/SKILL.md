---
name: ci-fix-go
description: Triage and fix failing Go CI jobs safely
argument-hint: "Paste failing CI logs or command output"
allowed-tools: Read,Write,Edit,Glob,Grep,Bash
---

CI triage protocol:
1) Classify failure type
- compile, test, race, lint, vuln, integration environment

2) Reproduce locally with the narrowest command.

3) Implement smallest safe fix.
- preserve behavior unless tests specify change
- add/adjust tests if behavior changes intentionally

4) Re-run failing stage, then full quality gates.

5) Summarize root cause and exact fix scope.

Never bypass checks with skip flags unless explicitly requested.
