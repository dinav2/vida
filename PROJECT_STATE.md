# vida - Project State

**Version:** 1.0.0
**Last Updated:** 2026-03-07

---

## 📊 Metrics

| Metric | Value |
|--------|-------|
| Total Specs | 1 |
| Completion Rate | 0% |
| Avg Cycle Time | 2 days |

### By Stage

| Stage | Count | Percentage |
|-------|-------|------------|
| DRAFT | 0 | 0% |
| SPEC | 0 | 0% |
| TEST-CREATE | 0 | 0% |
| TEST-APPLY | 0 | 0% |
| VERIFY | 1 | 100% |
| DONE | 0 | 0% |
| ARCHIVED | 0 | 0% |

---

## 📋 Active Specs

| ID | Title | Stage | Priority | Updated |
|----|-------|-------|----------|---------|
| SPEC-20260305-001 | vida — AI-Native Command Palette for Wayland (MVP Core) | VERIFY | P1 | 2026-03-07 |

---

## 🗂️ Spec Details

### SPEC-20260305-001 · VERIFY
**Title:** vida — AI-Native Command Palette for Wayland (MVP Core)
**Spec:** `specs/active/SPEC-20260305-001.md`
**Next step:** `/specsafe-done` — archive spec and tag release

**Verify Results (2026-03-07):**
| Check | Result | Notes |
|-------|--------|-------|
| `go test ./internal/... ./cmd/vida-daemon/` | ✅ 75 PASS, 1 SKIP, 0 FAIL | SCN-14 skipped (API keys absent, per TR-10f) |
| `go vet ./...` | ✅ Clean | No issues |
| `make build` | ✅ 3 binaries | vida, vida-daemon, vida-ui |
| AC-P2: RSS ≤ 30 MB | ✅ PASS | TestDaemon_IdleRSS passes |
| AC-P3: socket ready ≤ 200 ms | ✅ PASS | TestDaemon_SocketReady passes |
| AC-R4: client disconnect | ✅ PASS | TestServer_ClientDisconnect passes |
| AC-I1/I2: AI streaming | ⏭ Manual | Requires ANTHROPIC_API_KEY / OPENAI_API_KEY |
| AC-P1: UI show ≤ 100 ms | ⏭ Manual | Requires Wayland display |

**Implementation Summary:**
| Package | Tests | Status |
|---------|-------|--------|
| `internal/config` | 7 | ✅ |
| `internal/calc` | 8 | ✅ |
| `internal/shortcuts` | 8 | ✅ |
| `internal/db` | 9 | ✅ |
| `internal/apps` | 9 | ✅ |
| `internal/ai` (claude + openai) | 15 | ✅ |
| `internal/ipc` | 8 | ✅ |
| `internal/router` | 7 | ✅ |
| `cmd/vida-daemon` (integration) | 5 | ✅ (1 skip) |
| `cmd/vida` | — | Builds OK (thin IPC wrapper) |
| `cmd/vida-ui` | — | Builds OK (GTK4 + layer-shell) |

**Test Coverage:**
| File | Package | Scenarios / FRs |
|------|---------|-----------------|
| `internal/calc/calc_test.go` | `calc_test` | SCN-04, SCN-05, SCN-06, FR-05, TR-04 |
| `internal/router/router_test.go` | `router_test` | FR-04a–e, priority chain, cancellation |
| `internal/apps/apps_test.go` | `apps_test` | SCN-09, SCN-10, SCN-11, FR-06, TR-05 |
| `internal/shortcuts/shortcuts_test.go` | `shortcuts_test` | SCN-07, SCN-08, FR-07 |
| `internal/ai/claude_test.go` | `ai_test` | SCN-12, SCN-15, SCN-19, FR-08 (Claude) |
| `internal/ai/openai_test.go` | `ai_test` | SCN-13, SCN-15, SCN-19, FR-08 (OpenAI) |
| `internal/ipc/ipc_test.go` | `ipc_test` | FR-03, SCN-20, TR-02, AC-R4 |
| `internal/db/db_test.go` | `db_test` | SCN-16, SCN-17, FR-09, TR-07 |
| `internal/config/config_test.go` | `config_test` | FR-10, FR-08c |
| `cmd/vida-daemon/integration_test.go` | `main_test` | SCN-01, SCN-14, SCN-18, FR-01, AC-P2, AC-P3 |

---

*This file is managed by SpecSafe*
