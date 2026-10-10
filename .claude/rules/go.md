---
paths: ['**/*.go', '**/go.mod']
---

# Go

Loaded when a Go file is read. Not resident.

Batch implementation until a production path is complete. Use a narrow build or vet
command earlier only to unblock work. At a completed change boundary, run the
smallest integration target that exercises the real package entry point and external
collaborators, commonly a repository selector with `-tags=integration`. Unit,
filtered-function, mock-only, and per-edit tests are not completion evidence. Reserve
broad race-detector and workspace gates for the final applicable phase or release.

## Hard rules

- Race detection costs 5-10x memory and 2-20x execution time, and only finds
  races on paths the test actually exercises. It is a final boundary gate, not a
  continuous check.
- Errors are values. Wrap with context at the boundary, do not swallow.

<!-- Replace example boundaries with this project's real production-path gates. -->
