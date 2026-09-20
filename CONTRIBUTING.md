# Contributing to Cliostra

Thanks for your interest in Cliostra.

Cliostra is currently in **design / pre-alpha**, so the most valuable contributions are not only code. Provider research, security review, MCP interoperability notes, platform discovery behavior, UX feedback, and architecture proposals are all useful.

## Before opening a large change

For changes that affect architecture, provider support, security boundaries, MCP contracts, job/session behavior, or configuration UX, open an issue first. The goal is to avoid implementing assumptions that later conflict with provider capabilities or account policies.

Small documentation fixes, typo corrections, tests, and clearly scoped maintenance changes do not need prior discussion.

## Contribution areas

Useful contribution areas include:

- AI agent discovery on Linux, macOS, and Windows;
- MCP host compatibility;
- provider adapter research;
- subprocess and long-running job supervision;
- session lifecycle and cancellation;
- TUI design and usability;
- capability detection;
- skills discovery and activation strategies;
- security and prompt-injection defenses;
- provider policy verification;
- documentation and examples;
- testing infrastructure.

## Provider integrations

A technically possible integration is not automatically acceptable.

Before an adapter is presented as supported, contributors should verify:

- an official or clearly supported programmatic interface exists;
- the integration does not extract or reuse private credentials;
- quotas, approvals, sandboxes, and safety controls are preserved;
- the intended use is compatible with the provider's current terms;
- the adapter can fail safely when capabilities change.

If policy or documentation is ambiguous, mark it as ambiguous rather than guessing.

## Development principles

Contributions should preserve these project principles:

- local-first orchestration;
- provider-agnostic core;
- MCP-first host interoperability;
- capability-driven adapters;
- least privilege;
- clean final-result handoff;
- minimal context leakage between agents;
- no quota or safety bypassing;
- cross-platform behavior;
- simple setup and clear failure modes.

## Pull requests

Keep pull requests focused. Include:

- what problem is being solved;
- what behavior changes;
- any provider documentation used to justify the change;
- security implications;
- platform-specific considerations;
- tests or validation steps when code exists.

For provider-specific behavior, link to the official documentation that supports the implementation.

## Commit style

Use concise Conventional Commits where practical:

- `feat:` new behavior;
- `fix:` bug fix;
- `docs:` documentation;
- `refactor:` internal restructuring;
- `test:` tests;
- `chore:` maintenance.

## Code of conduct

Participation in Cliostra requires respectful, technical, good-faith collaboration. See [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md).

## License

By contributing to Cliostra, you agree that your contributions are licensed under the MIT License used by this repository.
