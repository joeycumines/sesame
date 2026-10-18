# Agent Operational Rules & Constraints

## Tooling & Runtime Constraints

- **Runtime Environments**:
  - **Development Environment = ONLY Bun**: All development workflow, package management, script execution, and tests MUST use `bun` and `bunx` exclusively.
  - **Supported Production Runtime = BOTH Node.js and Bun**: The built package and command binary (`sesame-endpoint`) MUST execute cleanly under both Node.js (via `#!/usr/bin/env node`) and Bun. Standard Node builtins (`node:net`, `node:tls`, `node:http2`, `node:stream`, `node:buffer`) ensure cross-runtime compatibility.
- **Strict Ban on `npm` and `npx`**:
  - **NEVER use `npx`**. Ever.
  - **NEVER use `npm`**. Ever.
  - **ALWAYS use `bun` and `bunx`** for all JavaScript/TypeScript package management, script execution, CLI execution, and testing.
  - Examples:
    - Package execution: `bunx <tool> [args]` (NEVER `npx`)
    - Dependency installation: `bun install` or `bun add` (NEVER `npm install` or `npm add`)
    - Running scripts: `bun run <script>` (NEVER `npm run`)
    - Testing: `bun test`

- **Build Tooling**:
  - Use GNU Make (`gmake` on macOS, `make` on Linux).
  - Target packages specifically to avoid slow test suites when running local checks.

## Project Architecture & Configuration Standards

- **Strict Ban on Config Files in Runtime Components**:
  - Runtime servers (such as `sesame-endpoint`) MUST NOT read configuration files (no YAML, JSON, TOML, `.env`).
  - Configuration MUST be parsed strictly from CLI flags and environment variables at entrypoint.
  - Environment variables for secrets MUST be carefully scoped (e.g. `SESAME_ENDPOINT_*`).
