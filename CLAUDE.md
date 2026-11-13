# CLAUDE.md

AWS FinOps Dashboard **v3** — a Go rewrite of the v2 Python/Rich CLI.
Design decisions and their rationale live in `OVERHAUL.md`. Read that before proposing changes.

## Before you write any Charm code — read this

**Charm v2 shipped 2026-07-03. Your training data is almost certainly v1-shaped, and v1 idioms
in this stack compile and misbehave rather than failing loudly.**

1. **Check `docs/STACK.md` first** — it records verified, version-stamped API facts, weighted
   toward things that type-check but do the wrong thing.
2. **If it isn't there, verify before use:** `go doc <pkg>.<Symbol>`, or read the real source in
   `$(go env GOMODCACHE)/charm.land/...`. Then add what you learned to `docs/STACK.md`.
3. **Never rely on recall for this stack.** A prior session got `Canvas.Compose` and
   `Model.View()` wrong from memory; both compiled cleanly.

The compiler will not save you here. Every real bug found during the stack spike was
**semantically wrong and fully type-checked**. Tests and golden frames are the safety net.

## Working agreement

- **Small increments**, each agreed as a mockup before implementation.
- **TDD for engine code** — cost/date arithmetic, pricing, check evaluators, formatters.
- **Golden-frame snapshots for TUI code** — written after the screen exists, not before.
- **`make verify` must pass** before anything is considered done.
- Dependency upgrades update `docs/STACK.md` in the same commit. No automated upgrade bots.

## Architecture

```
cmd/finops/        entry point, TTY detection, exit codes
internal/core/     credential resolution, account model
internal/audit/    check declarations + evaluators
internal/engine/   scan planner, API call dedup, query execution
internal/exitcode/ the exit taxonomy (agent + CI contract)
internal/render/   tui | plain | json | markdown
internal/sink/     stdout | file | s3 | slack
examples/          runnable, verified demos of tricky APIs
```

Tests live beside the code they test. TUI golden frames use
`charmbracelet/x/exp/teatest/v2` — there is no separate test harness and no
Python dependency; `go test ./...` runs everything.

## Non-negotiables

- **Read-only.** The tool never mutates AWS resources.
- **TTY-gated interactivity.** Non-TTY stdout ⇒ static, ANSI-free, no banner, no spinner.
  `--json`, `--format`, and `--out` never render a TUI.
- **Cost metric is a required field** on any rendered figure — never let a number appear
  without stating whether it's unblended, amortized, or net amortized.
- **Cost Explorer calls cost real money.** Estimate before spending, cache aggressively,
  and never make a billable call the default action.
- **Errors are road signs** — every failure states what happened and the next command to run.
- **Declare once, derive the rest.** Keybindings generate their own help; audit checks generate
  the catalogue, settings, scan plan, and docs. Don't hand-maintain parallel lists.

## Commands

```
make verify     build + vet + test + golden-frame diff     ← must pass
make test       unit tests only
make golden     regenerate golden frames (review the diff!)
make run        build and run locally
```

## Reference

- `OVERHAUL.md` — requirements, decisions, and rationale
- `docs/STACK.md` — verified API facts
- `examples/` — working code for the APIs that are easy to get wrong
- v2 Python source is still in the tree; it is the porting oracle for cost/date semantics.
