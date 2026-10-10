---
paths: ['**/*.ts', '**/*.tsx', '**/package.json', '**/tsconfig.json']
---

# TypeScript

Loaded when a TypeScript file is read. Not resident.

Batch implementation until a production path is complete. Use a narrow type check
earlier only when it is needed to unblock work. At a completed change boundary, run
the smallest browser, API, process, or build integration that exercises the real
entry point and collaborators. Unit, component-only, snapshot, and per-edit tests are
not completion evidence. Reserve broad end-to-end, visual, and bundle gates for the
final applicable phase or release boundary.

## Hard rules

- Bun and esbuild strip types without checking them. `tsc --noEmit` is the real
  type gate — a green Bun run proves nothing about types.
- Cache `.tsbuildinfo`. Incremental typecheck drops substantially with it.
- Watch mode and per-edit test loops are not completion gates.
- Keep e2e to the flows where failure costs money, not to everything reachable.

## Structure

Organize by capability under `features/<domain>/`, not by technical layer.
Layer order is UI, then hooks, then stores, then services, then external. A
component does not call a service or mutate a store directly.

Components render and submit intent. No business rule exists only in a
component. No browser storage in artifacts.

<!-- Replace example boundaries with this project's real production-path gates. -->
