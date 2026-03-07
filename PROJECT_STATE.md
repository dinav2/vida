# vida - Project State

**Version:** 1.0.0
**Last Updated:** 2026-03-07

---

## 📊 Metrics

| Metric | Value |
|--------|-------|
| Total Specs | 2 |
| Completion Rate | 50% |
| Avg Cycle Time | 2 days |

### By Stage

| Stage | Count | Percentage |
|-------|-------|------------|
| DRAFT | 0 | 0% |
| SPEC | 0 | 0% |
| TEST-CREATE | 1 | 50% |
| TEST-APPLY | 0 | 0% |
| VERIFY | 0 | 0% |
| DONE | 1 | 50% |
| ARCHIVED | 0 | 0% |

---

## 📋 Active Specs

| ID | Title | Stage | Priority | Updated |
|----|-------|-------|----------|---------|
| SPEC-20260307-002 | vida Search Input Wiring | TEST-CREATE | P1 | 2026-03-07 |

---

## ✅ Completed Specs

| ID | Title | Completed | Tests |
|----|-------|-----------|-------|
| SPEC-20260305-001 | vida — AI-Native Command Palette for Wayland (MVP Core) | 2026-03-07 | 75 pass, 1 skip |

---

## 🗂️ Spec Details

### SPEC-20260307-002 · TEST-CREATE
**Title:** vida Search Input Wiring
**Spec:** `specs/active/SPEC-20260307-002.md`
**PRD:** `specs/drafts/SPEC-20260307-002.md`
**Next step:** `/specsafe-test-apply` — implement until all tests pass

**Test Coverage:**
| File | Package | Scenarios / FRs |
|------|---------|-----------------|
| `internal/debounce/debounce_test.go` | `debounce_test` | FR-01d, FR-07b — debounce timer |
| `cmd/vida-daemon/stream_test.go` | `main_test` | SCN-01, SCN-02, SCN-06–09, TR-02, TR-05 |

**Test count:** 12 test functions
**RED state:**
- `internal/debounce` — package not yet created (build fails)
- `TestQuery_AIStreaming` — daemon sends `result` not `token` messages (FAIL)
- `TestQuery_CancelAI/NewQueryCancelsOld/CancelUnknownID` — pass with soft assertions (will tighten after streaming impl)

**UI tests:** GTK4 wiring (FR-02, FR-03, FR-04, FR-05) verified manually on Wayland

---

*This file is managed by SpecSafe*
