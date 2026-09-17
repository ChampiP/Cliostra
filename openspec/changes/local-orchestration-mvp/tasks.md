# Tasks: local-orchestration-mvp

Strict TDD: each task writes/extends the test file first (or in the same commit) before the implementation it verifies.

## 1. Module scaffold
- [x] 1.1 `go.mod` (module `github.com/ChampiP/Cliostra`, Go version, pin `github.com/modelcontextprotocol/go-sdk/mcp`).

## 2. internal/api (DTOs, error codes, RPC framing)
- [x] 2.1 `internal/api/api_test.go`: encode/decode JSON-lines frames, 256KiB limit rejection, error code shape.
- [x] 2.2 `internal/api/api.go`: DTOs (StartRequest/Response, StatusResponse, ResultResponse, CancelResponse), error codes, line-delimited JSON client/server over `net.Conn`.

## 3. internal/runtime — state machine
- [x] 3.1 `internal/runtime/state_test.go`: valid/invalid transitions (`queued→preparing→running→succeeded|failed`, `running→canceling→canceled`), idempotent re-application.
- [x] 3.2 `internal/runtime/state.go`: `Job` struct, `State` type, transition function.

## 4. internal/runtime — durable persistence
- [x] 4.1 extend `runtime_test.go`: atomic write (temp+fsync+rename+dir fsync), reload after simulated crash mid-write, restart marks non-terminal jobs `failed/daemon_restarted`.
- [x] 4.2 `store.go` + `runtime.go`: JSON persistence under `XDG_STATE_HOME`, restart recovery pass.

## 5. internal/runtime — worker pool scheduler
- [x] 5.1 extend `runtime_test.go`: bounded concurrency (N workers respected), backlog stays `queued` under load, `start` returns before execution.
- [x] 5.2 extend `runtime.go`: cola en memoria (sin límite fijo) + configurable N worker goroutines feeding the state machine.

## 6. internal/runtime — worktrees, process limits, cancellation
- [x] 6.1 extend `runtime_test.go` with temp git repo helper: worktree add at detached HEAD OID, permission strip/restore, cleanup on completion; prompt/stream/result size limits (64KiB/8MiB/1MiB) truncate and fail; cancel only transitions after process exit (TERM→KILL).
- [x] 6.2 `git.go` + `runtime.go`: `git worktree add --detach`, process supervision, limit enforcement, cancellation flow.

## 7. internal/adapters
- [x] 7.1 `internal/adapters/adapters_test.go`: argv construction per adapter, capability flags, `agy` cancel → `unsupported`, no env/secret leakage assertions.
- [x] 7.2 `internal/adapters/adapters.go`: `Adapter` interface, `ProcessSpec`, Claude Code adapter, `agy` adapter (fixed argv, stdin-only prompt).

## 8. internal/mcp
- [x] 8.1 `internal/mcp/server_test.go`: four tools (start/status/result/cancel) over in-memory transport, contract shape, zero real providers.
- [x] 8.2 `internal/mcp/server.go`: MCP server wiring tools to the RPC client.

## 9. cmd/cliostrad
- [x] 9.1 `cmd/cliostrad/main.go`: socket listener (`0600`, `XDG_RUNTIME_DIR`), wires runtime+adapters, restart recovery on boot.

## 10. cmd/cliostra
- [x] 10.1 `cmd/cliostra/main.go`: `start|status|result|cancel` subcommands (RPC client) + MCP stdio mode entrypoint.

## 11. Wire-up smoke test
- [x] 11.1 `internal/integration/smoke_test.go`: boot daemon in-process, `start` a trivial read-only adapter call, poll `status`, fetch `result` — proves CLI/MCP/runtime/adapter/socket all connect end to end.

## Review Workload Forecast

| Group | Est. changed lines | 400-line risk |
|---|---|---|
| 1–2 (scaffold, api) | ~180 | Low |
| 3–6 (runtime, all parts) | ~520 | High |
| 7 (adapters) | ~150 | Low |
| 8 (mcp) | ~140 | Low |
| 9–11 (cmd + smoke) | ~160 | Low |

- 400-line budget risk: **High** for the runtime group (3–6) alone.
- Chained PRs recommended: **Yes**, split runtime into its own chain (state → persistence → scheduler → worktrees/process) rather than one PR.
- Decision needed before apply: **Yes** — delivery strategy is `ask-on-risk`, so confirm chaining before `sdd-apply` runs the runtime group.
