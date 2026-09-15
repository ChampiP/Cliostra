# Security Policy

## Project status

Cliostra is currently in **design / pre-alpha**. There is no stable release yet.

Security is a core design constraint because Cliostra is intended to launch AI agents, grant them scoped access to local workspaces, and mediate tool execution across multiple providers.

## Reporting a vulnerability

Please do **not** publish an exploit, credential exposure, sandbox escape, arbitrary command execution issue, or provider-account abuse path in a public issue before maintainers have had a chance to assess it.

Until a dedicated private reporting channel is published, contact the repository owner through GitHub and request a private security contact path. Public issues are appropriate for non-sensitive hardening suggestions and design discussions that do not disclose an active vulnerability.

## High-priority security areas

Reports are especially valuable when they involve:

- arbitrary shell execution outside intended controls;
- workspace escape or path traversal;
- credential, token, cookie, or keyring exposure;
- unsafe environment-variable inheritance;
- prompt injection that changes Cliostra authorization decisions;
- worker output escalating its own permissions;
- MCP tool poisoning or capability spoofing;
- unintended network exposure;
- unsafe job cancellation or orphaned processes;
- cross-session data leakage;
- logs containing prompts, secrets, or source code unexpectedly;
- provider-policy bypass behavior;
- account-sharing or quota-evasion behavior enabled by the runtime.

## Security principles

Cliostra is expected to follow these principles:

- official provider interfaces only;
- no credential extraction;
- least-privilege worker execution;
- explicit workspace boundaries;
- capability-driven adapters;
- separation of result output from operational logs;
- untrusted treatment of worker-generated content;
- no permission escalation based on model output;
- conservative defaults for background jobs;
- provider policy validation before stable support.

For the broader project posture, see [`docs/SECURITY_AND_POLICY.md`](docs/SECURITY_AND_POLICY.md).
