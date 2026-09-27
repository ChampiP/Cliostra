# List active Cliostra jobs

## Goal
Expose a read-only MCP tool that lets a host inspect currently active Cliostra jobs, addressing the inability to discover a job ID after a timed-out `run` call.

## Scope
- Add a durable runtime/API listing path backed by the existing persisted job store.
- Expose active jobs through a new MCP tool; do not add provider-session enumeration, CLI command, cancellation, or result retrieval behavior.
- Return safe job summaries only; never include prompts, results, or diffs.
- Define active as every nonterminal state (`queued`, `preparing`, `running`, `canceling`), and make ordering deterministic.

## Acceptance criteria
1. The MCP list tool returns persisted active-job summaries with identifiers and useful metadata, and returns an empty list when no jobs are active.
2. Terminal jobs (`succeeded`, `failed`, `canceled`) are excluded.
3. Prompt/result content is not exposed.
4. Focused tests cover active/terminal filtering and MCP response behavior.

## Tasks
1. Add a deterministic active-job list operation over persisted store records and its API/RPC contract.
2. Expose the operation as a read-only MCP tool and add focused tests.
3. Run focused verification and inspect the final diff.

## Evidence
- Exploration: `Store.LoadAll()` already enumerates durable job records; no runtime/API/CLI/MCP listing endpoint exists.
- Current user intent: add a Cliostra MCP tool to discover active work, especially after a caller timeout without a returned job ID.

## Status
- Task 1 complete: runtime exposes deterministic persisted nonterminal-job summaries through the `list_active` RPC.
- Task 2 complete: MCP exposes `list_active_jobs`; focused runtime and MCP tests pass.
- Task 3 in progress: independent verification and final candidate review pending.

## Implementation evidence
- Added `Runtime.ListActive`, filtering terminal states and sorting by job ID.
- Added safe `ActiveJob`/`ListActiveResponse` API DTOs and the `list_active` RPC handler.
- Added the read-only MCP `list_active_jobs` tool.
- Added runtime tests for active filtering/order and MCP coverage for an empty response.
- Writer verification: `go test ./internal/runtime ./internal/mcp` passed; `git diff --check` passed.
