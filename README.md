<div align="center">

# Cliostra

### Universal subagent orchestration for AI coding CLIs

**Turn the AI agents you already have installed and authenticated into interoperable subagents — through MCP, isolated sessions, and clean final-result handoffs.**

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Project Status](https://img.shields.io/badge/status-design%20%2F%20pre--alpha-orange.svg)](#project-status)
[![MCP](https://img.shields.io/badge/protocol-MCP-7c3aed.svg)](https://modelcontextprotocol.io/)
[![Go](https://img.shields.io/badge/core-Go-00ADD8.svg?logo=go&logoColor=white)](https://go.dev/)

**AI agent orchestration · MCP · CLI agents · subagents · multi-provider · local-first · context-efficient**

</div>

---

## What is Cliostra?

Cliostra is an open-source project for **orchestrating AI agents that are already installed on your machine**.

The goal is to let any compatible AI coding agent act as a **host** and delegate work to other local AI agents as if they were native subagents. A host should be able to choose a target agent, select a model when supported, define a workspace, provide a focused prompt, start or resume an isolated session, let the delegated agent work, and receive only the final useful result.

Cliostra is designed around a simple idea:

> **Use the AI subscriptions and local agents you already have as specialized workers, without forcing the main agent to carry every exploration, tool call, log, and intermediate step in its own context.**

This is not intended to replace Claude Code, OpenCode, Codex, Antigravity, or other coding agents. Cliostra aims to make them **interoperable**.

## Why Cliostra?

Modern coding agents already know how to delegate work to internal subagents. The limitation is that those subagents usually live inside the same product or provider.

At the same time, many developers already pay for or use multiple AI coding tools. Each tool may have different models, quotas, strengths, reasoning modes, tools, skills, and context windows.

Cliostra aims to bridge those worlds:

- delegate research to one agent while the main agent keeps working;
- use a different subscription for a long-running analysis task;
- keep delegated exploration out of the main agent's context window;
- run each worker inside its own session and workspace;
- receive a clean final result instead of execution noise;
- choose which agents are hosts, workers, or both;
- expose the orchestration layer through MCP so it is not tied to one vendor.

The intended benefit is **better use of existing subscriptions and better context efficiency**, not quota bypassing or provider impersonation.

## Core concept

Cliostra treats AI agents as interchangeable roles rather than fixed products.

An agent may be:

- **Host** — consumes Cliostra through MCP and delegates work.
- **Worker** — receives a delegated task through an officially supported programmatic interface.
- **Both** — orchestrates in one workflow and can be orchestrated in another.

For example, Claude Code could delegate to Codex in one session, while OpenCode could delegate to Claude Code in another. Cliostra should not hard-code a permanent hierarchy between providers.

## How it works

Cliostra operates as a lightweight local system with three primary layers:

1. **`cliostrad` daemon**: A per-user background daemon listening exclusively on a private Unix domain socket (`0600`) under `$XDG_RUNTIME_DIR/cliostra/cliostrad.sock` (or `~/.local/state/cliostra/`). It persists durable job records (`queued`, `preparing`, `running`, `succeeded`, `failed`, `canceled`) in `$XDG_STATE_HOME/cliostra/jobs`, coordinates a worker pool, recovers unfinished jobs on restart, and runs background MCP synchronization (e.g. for `agy`).
2. **`cliostra` CLI & MCP server**: A client binary that communicates with `cliostrad` via Unix socket RPC. It provides CLI commands (`start`, `status`, `result`, `cancel`) and runs an MCP server over stdio via `cliostra mcp` exposing tools: `run`, `status`, `result`, `wait`, `cancel`, and `delegate`.
3. **Provider adapters (`internal/adapters`)**: Concrete process builders for installed AI CLI tools (`claude-code`, `agy`, `codex`, `opencode`) using fixed argument vectors and stdin (no shell execution, no credential extraction).

```mermaid
flowchart LR
    USER[Developer]

    subgraph HOST["MCP Host"]
        H["Claude Code / OpenCode / Codex / other"]
    end

    subgraph CLIENT["Cliostra Client"]
        CLI["cliostra CLI (start / status / result / cancel)"]
        MCP["cliostra mcp (run / wait / status / result / cancel / delegate)"]
    end

    subgraph DAEMON["cliostrad Daemon"]
        SOCK["Unix Domain Socket (0600)"]
        RPC["RPC Handler"]
        RUNTIME["Runtime Engine (Queue & Workers)"]
        STORE["Durable Store (XDG_STATE_HOME)"]
    end

    subgraph ADAPTERS["Worker Adapters (internal/adapters)"]
        W1["claude-code (claude CLI)"]
        W2["opencode (opencode run)"]
        W3["codex (codex exec)"]
        W4["agy (Antigravity NDJSON)"]
    end

    subgraph NOTIFY["Push Notification"]
        UDS["Claude Code Messaging Socket"]
    end

    USER --> CLI
    H -->|stdio MCP| MCP
    CLI -->|Unix socket RPC| SOCK
    MCP -->|Unix socket RPC| SOCK
    SOCK --> RPC
    RPC --> RUNTIME
    RUNTIME <--> STORE
    RUNTIME --> ADAPTERS
    ADAPTERS -->|execute in shared repo root| USER
    MCP -.->|background completion notice| UDS
    UDS -.->|injected user message| H
```

### Execution model

1. **Task initiation**:
   - **Via MCP**: A host calls `run` (synchronous, blocking until completion or timeout) or `delegate` (asynchronous, non-blocking).
   - **Via CLI**: The developer calls `cliostra start --adapter <name> --repo <path> --prompt <text> [--write] [--model <m>] [--effort <e>]`.
2. **Durable queue & supervision**:
   - `cliostrad` accepts the job, registers it in the durable store, and dispatches it to an available worker goroutine.
   - If the daemon crashes or restarts, jobs in non-terminal states are recovered.
3. **Adapters & repository execution**:
   - The assigned adapter generates a structured execution specification (`ProcessSpec`) without shell interpolation.
   - **Shared repository execution vs. worktree isolation**: While `internal/runtime/worktree.go` includes helpers for disposable git worktrees, all four current adapters (`claude-code`, `agy`, `codex`, `opencode`) are configured as `directRepoAdapters` and run in the shared repository root (`gitToplevel`). This ensures:
     - `agy` commands are not derailed by unreliable directory isolation in headless mode;
     - `codex` can access unstaged or untracked working files;
     - Developers and host agents can inspect changes in real time using `git diff`.
   - `read_only` mode is enforced logically through provider flags and sandboxes (e.g., `plan` mode for Claude, tool restriction prompts for Antigravity, `--sandbox read-only` for Codex, and permission denial via `OPENCODE_CONFIG_CONTENT` for OpenCode), rather than filesystem permission stripping.
4. **Result handoff & notifications**:
   - Once the process terminates, the adapter parses structured output (JSON, JSONL, or NDJSON events) to extract the clean final result.
   - For synchronous callers (`run`, `wait`, or `cliostra result`), the result is returned directly.
   - For asynchronous callers in Claude Code (`delegate`), Cliostra uses `internal/notify` to deliver the outcome directly into the active session via Claude Code's messaging socket.

The main agent does **not** receive the worker's full execution trace by default.

## Context-efficient delegation

Cliostra is intentionally designed around **context isolation**.

A delegated worker may inspect many files, invoke tools, perform searches, reason across a large context, or run for several minutes. The host usually does not need all of that information.

By default, the desired handoff is:

```text
Host -> focused task -> Cliostra -> external worker
Host <- final useful result <- Cliostra <- external worker
```

Not:

```text
Host <- every log, tool event, intermediate step and worker transcript
```

This does not eliminate the worker's own token or quota usage. The purpose is to **distribute work across available agents and reduce unnecessary context consumption in the host**.

## Long-running subagents & async notifications

Delegated tasks can range from quick queries to multi-minute code changes. Cliostra supports both synchronous and asynchronous coordination patterns:

- **Synchronous (`run` / `wait`)**: The host invokes `run`, which blocks on the server side until the worker finishes or a timeout expires (default 60s, up to 10 minutes). If a timeout is reached, the job continues running in `cliostrad`, and the host can resume waiting with `wait --id <id>`.
- **Asynchronous push notifications (`delegate`)**: When running inside Claude Code (`CLAUDE_CODE_MESSAGING_SOCKET` and `CLAUDE_CODE_MESSAGING_TOKEN`), the host can invoke `delegate`. This tool enqueues the job and immediately returns the job ID. A background goroutine in `cliostra mcp` waits for completion (up to 2 hours) and automatically posts a notification into Claude Code's session via the Unix messaging socket with the job ID, adapter, terminal state, final result, and diff (capped at 4,000 characters). The host never needs to poll `status` or manage sleep loops.
- **Durable polling (`status` / `result`)**: Any client or host can query durable job state or retrieve the final result at any time using the job ID.

## Agent discovery

A major product goal is **near-zero configuration**.

Cliostra should be able to discover local AI tooling and distinguish between:

- installed;
- detected but not automatable;
- programmatic interface available;
- capabilities known;
- policy validated;
- enabled as a worker;
- enabled as an MCP host.

Detection may use PATH lookup, known installation locations, configuration directories, desktop application metadata, or capability probes depending on the product and platform.

Detecting an application does **not** automatically mean Cliostra is allowed or technically able to orchestrate it.

## TUI configuration experience

Cliostra is planned to include an interactive terminal UI so setup feels closer to:

```text
install -> detect -> select -> connect -> use
```

The TUI is expected to help users:

- discover installed AI agents;
- see which ones are actually orchestrable;
- choose which agents can act as hosts;
- choose which agents can act as workers;
- inspect capabilities supported by each integration;
- configure MCP connections without manually editing multiple files;
- validate connectivity;
- inspect available skills and optional agent capabilities.

The TUI is a control plane for configuration. It should not be required for normal runtime operation.

## Skills and specialized workers

Cliostra is also exploring **skill-aware delegation**.

The idea is to detect skills already installed for compatible agents and allow users to associate selected skills with selected workers or task profiles. When a delegated task starts, Cliostra could activate only the relevant capability if the target agent supports doing so safely and efficiently.

This is still under evaluation. Automatically injecting complete skill content into every task may increase context usage and work against the project's primary goal. Cliostra will prefer native provider mechanisms when available and should load only what is necessary.

## Cross-platform goal

Cliostra is intended to support:

- Linux
- macOS
- Windows

The core is written in **Go** because the project benefits from a lightweight native binary, process supervision, concurrency primitives, and straightforward cross-platform distribution.

The TUI framework is not locked yet. Bubble Tea is being evaluated because it is a mature Go TUI ecosystem and fits the single-binary direction.

## MCP-first, vendor-agnostic

Cliostra uses MCP as the interoperability layer for hosts because the project should not depend on one specific coding agent.

Any host that can consume the required MCP tools should be able to use Cliostra. Worker integrations are separate adapters because each AI agent exposes different capabilities, session semantics, model selection, permission systems, and automation surfaces.

Cliostra therefore separates:

- **host interoperability** through MCP;
- **worker compatibility** through provider-specific adapters;
- **capability discovery** so unsupported features are not assumed;
- **policy validation** so a technically possible integration is not automatically considered acceptable.

## Adapter execution: shared repo vs. worktrees

While the Cliostra runtime includes git worktree isolation utilities (`prepareWorktree` / `cleanupWorktree`), all four current adapters—**Claude Code**, **Antigravity (`agy`)**, **Codex**, and **OpenCode**—run directly in the shared repository root (`directRepoAdapters`):

- **Tool confinement**: CLI agents like `agy` do not reliably constrain their tool/command execution to secondary worktree paths in headless mode.
- **Context visibility**: Tools like `codex` need access to untracked or staged files that a clean `HEAD` worktree would hide.
- **Live inspection**: Executing on the shared working copy allows developers to monitor changes in real time via `git diff` without waiting for a detached diff review handoff.

`read_only` is an instruction and permission restriction passed directly to the provider/sandbox, not structural filesystem isolation.

## Security and provider policies

Cliostra is built around official local interfaces, not credential extraction.

The project will not intentionally:

- copy or export provider access tokens;
- scrape authenticated web applications;
- impersonate provider APIs;
- bypass quotas, rate limits, approvals, or safety controls;
- rotate accounts to evade limits;
- expose a personal subscription as a shared public service;
- silently grant workers unrestricted shell, network, or filesystem access.

Each provider integration must be reviewed independently because technical automation support and account terms are not always equivalent.

See [`docs/SECURITY_AND_POLICY.md`](docs/SECURITY_AND_POLICY.md) for the project's security and provider-policy posture.

## Supported & planned integrations

The following agents are part of the orchestration scope:

| Agent | Host role | Worker role | Current status |
| --- | --- | --- | --- |
| Claude Code | MCP host & async socket notification | CLI adapter (`claude --print --permission-mode`) | Implemented (`internal/adapters/claude.go`) |
| OpenCode | MCP host & plugin (`plugins/opencode`) | CLI adapter (`opencode run --pure`) | Implemented (`internal/adapters/opencode.go`) |
| OpenAI Codex | MCP host | CLI adapter (`codex exec --json`) | Implemented (`internal/adapters/codex.go`) |
| Google Antigravity | MCP host | CLI adapter (`agy` NDJSON stream) | Implemented (`internal/adapters/agy.go`) |
| Other MCP hosts | Candidate via stdio MCP | Custom adapter | Extensible via `internal/adapters` |

## Project status

Cliostra is currently in **design / pre-alpha**.

The repository intentionally starts documentation-first. The architecture, trust boundaries, provider policies, long-running job behavior, and user experience are being defined before implementation so the first code does not lock the project into an unsafe or provider-specific design.

There is no stable release or installation command yet.

## Design principles

Cliostra follows a small set of project-level principles:

- **Local-first** — orchestration happens on the user's machine.
- **Provider-agnostic** — no provider is permanently the orchestrator or worker.
- **MCP-first for hosts** — interoperability should be standard, not bespoke.
- **Capability-driven** — adapters advertise what they actually support.
- **Context-efficient** — return the useful result, not execution noise.
- **Subscription-aware, not quota-evading** — use legitimate access without bypassing provider controls.
- **Least privilege** — workers receive only the permissions required for the task.
- **Clean setup** — configuration should be discoverable and interactive rather than manual and fragile.
- **Cross-platform** — Linux, macOS, and Windows are first-class targets.
- **Open-source extensibility** — adding a new agent should be an adapter problem, not a core rewrite.

## Documentation

- [`docs/PROJECT_VISION.md`](docs/PROJECT_VISION.md) — product vision, orchestration model, TUI goals, discovery, jobs, sessions, and skills.
- [`docs/SECURITY_AND_POLICY.md`](docs/SECURITY_AND_POLICY.md) — account safety, provider terms, permissions, credentials, prompt injection, and integration criteria.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — how to contribute.
- [`SECURITY.md`](SECURITY.md) — how to report security issues.

## Contributing

Cliostra is intended to grow as a community project. Useful contributions include provider research, adapter design, MCP interoperability, Windows/macOS/Linux discovery logic, TUI UX, security review, tests, documentation, and policy verification.

Please read [`CONTRIBUTING.md`](CONTRIBUTING.md) before opening implementation work. Since the project is pre-alpha, major architectural changes should begin as a discussion or issue before large code contributions.

## License

Cliostra is licensed under the [MIT License](LICENSE).

## Disclaimer

Cliostra is an independent open-source project and is not affiliated with, endorsed by, or sponsored by Anthropic, OpenAI, Google, OpenCode, or other AI-agent providers mentioned in this repository. Product names and trademarks belong to their respective owners.

Users remain responsible for complying with the terms, quotas, data policies, and acceptable-use rules of every provider they connect.

---

<div align="center">

**Cliostra** — make your AI agents work together without making one agent carry everything.

</div>
