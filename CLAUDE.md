# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# tmux-agent-deck

A terminal UI for managing multiple AI coding agent sessions in tmux, organized into nested groups.

## What This Is

tmux-agent-deck lets you run many AI coding agents (Claude, Gemini, etc.) in parallel tmux sessions and monitor them all from a single TUI. Sessions are grouped hierarchically. Status is detected automatically by reading tmux pane output in the background.

## Tech Stack

- **CLI:** [Cobra](https://github.com/spf13/cobra)
- **TUI:** [Bubbletea](https://github.com/charmbracelet/bubbletea) + [Lipgloss](https://github.com/charmbracelet/lipgloss)
- **DB:** SQLite via [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (pure Go, no CGO)
- **Language:** Go 1.22+

## Architecture

```
tmux-agent-deck/
├── main.go
├── cmd/
│   ├── root.go          # cobra entrypoint, launchTUI() loop, openDB()
│   ├── root_test.go     # headless mode + root flag tests
│   ├── add.go           # `add` subcommand
│   ├── list.go          # `list` subcommand
│   ├── remove.go        # `remove` subcommand
│   ├── session.go       # `session start/stop/attach` subcommands
│   ├── group.go         # `group create/delete/move` subcommands
│   └── cmd_test.go      # integration tests via RunWith()
├── internal/
│   ├── db/
│   │   ├── db.go        # Open(), migrate(), WAL mode + busy_timeout
│   │   ├── db_test.go
│   │   ├── groups.go    # Group type + CRUD
│   │   ├── groups_test.go
│   │   ├── sessions.go  # Session type + CRUD
│   │   └── sessions_test.go
│   ├── tmux/
│   │   ├── client.go    # Client, ClientIface, binding helpers, ParseBindingCommand
│   │   ├── status.go    # DetectStatus() pure function
│   │   └── status_test.go
│   ├── state/
│   │   ├── poller.go    # Poller: Start/Stop/PollOnce, TmuxReader interface, configurable interval
│   │   └── poller_test.go
│   ├── notify/
│   │   ├── notify.go    # Notification policy + osascript integration
│   │   └── notify_test.go
│   ├── ui/
│   │   ├── app.go       # Bubbletea Model, Init/Update/View, Reload(), help/empty-state rendering
│   │   ├── app_test.go
│   │   ├── list.go      # ListItem, BuildTree(), RenderList()
│   │   ├── list_test.go
│   │   ├── dialog.go    # dialogState, updateDialog(), commitDialog()
│   │   └── keys.go      # actionForKey() mapping + exported help table
│   └── testutil/
│       ├── db.go        # OpenTestDB(t) helper
│       └── tmux.go      # FakeTmuxClient for tests
```

**Data flow:** `poller` reads tmux pane output on a configurable interval (`--poll`) → writes status to DB → optionally emits notification events via `internal/notify` → `app` reads DB on tick and re-renders, or `--headless` blocks on the poller without launching Bubble Tea.

## Key Design Decisions

### TUI attach flow
The TUI quits (`tea.Quit`) when the user attaches to a session, then `launchTUI()` in `cmd/root.go` loops — after `AttachSession` returns it re-launches the TUI. This is how "return to TUI" works without suspending/resuming bubbletea.

### Return-to-TUI keybinding
When attaching from outside tmux, `ctrl+q` (`C-q` in the tmux `root` table) is temporarily bound to `detach-client`. The original binding is saved before attach and restored in a `defer` after attach returns. Implemented in `internal/tmux/client.go` `AttachSession()`.

### SQLite concurrency
`db.Open()` sets `conn.SetMaxOpenConns(1)`, `PRAGMA journal_mode=WAL`, and `PRAGMA busy_timeout=5000` to prevent `SQLITE_BUSY` errors from the poller goroutine and TUI competing for the DB.

### ClientIface
All tmux operations go through `tmux.ClientIface`. Tests use `testutil.FakeTmuxClient` — never mock the real tmux binary in tests.

### Schema migrations
Sequential version checks in `migrate()` using a `metadata` table (`key=schema_version`). Current version: 6. WAL/busy_timeout pragmas run on every open before migration.

## Development Workflow

This project uses a brainstorm → spec → plan → implement cycle:

1. **Brainstorm** (`/brainstorm` or `superpowers:brainstorming`) — design discussion, produces a spec doc
2. **Spec** saved to `docs/superpowers/specs/YYYY-MM-DD-<feature>-design.md`
3. **Plan** (`superpowers:writing-plans`) saved to `docs/superpowers/plans/YYYY-MM-DD-<feature>.md`
4. **Implement** via `superpowers:subagent-driven-development` — one subagent per task, spec + quality review after each

## Coding Conventions

- No comments unless the WHY is non-obvious (hidden constraint, subtle invariant, workaround)
- No error handling for scenarios that can't happen — trust internal code and framework guarantees
- No feature flags, backwards-compat shims, or half-finished implementations
- TDD: write failing test first, then implement
- Frequent small commits per task
- All tests in `*_test.go` files using `package <pkg>_test` (black-box)
- Integration tests use `testutil.OpenTestDB(t)` — never `:memory:` directly

## Commands

Build:
```bash
go build -o tmux-agent-deck .
```

Run all unit tests:
```bash
go test ./...
```

Run a single test:
```bash
go test ./internal/ui/ -run TestBuildTree
```

Run e2e tests (requires tmux on PATH; builds the binary automatically):
```bash
go test -tags e2e ./test/e2e/
```

Vet:
```bash
go vet ./...
```

## Roadmap

Full roadmap: [docs/superpowers/specs/2026-05-10-roadmap.md](docs/superpowers/specs/2026-05-10-roadmap.md)

| Milestone | Summary | Status | Spec | Plan |
|-----------|---------|--------|------|------|
| MVP / Split Panel TUI | Core TUI, groups, session list | complete | [spec](docs/superpowers/specs/2026-05-09-tmux-agent-deck-design.md) · [split panel](docs/superpowers/specs/2026-05-09-split-panel-tui-design.md) | [plan](docs/superpowers/plans/2026-05-10-split-panel-tui.md) |
| M1 Interaction Primitives | Send to pane (`x`), Fork (`f`), Broadcast (`b`) | complete | [spec](docs/superpowers/specs/2026-05-10-m1-interaction-primitives-design.md) | [plan](docs/superpowers/plans/2026-05-10-m1-interaction-primitives.md) |
| M2 Observability | Context window %, waiting timers, desktop notifications | complete | [spec](docs/superpowers/specs/2026-05-13-m2-finish-observability-design.md) | [plan](docs/superpowers/plans/2026-05-13-m2-finish-observability.md) |
| M3 Conductors + macOS Alerts | Group conductor (`c`), escalate (`C`), digest, quiet hours | complete | — | [plan](docs/superpowers/plans/2026-05-12-m3-conductors-macos-alerts.md) |
| M4 Session Configuration | Project path picker, tool selection, startup scripts | complete | — | [plan](docs/superpowers/plans/2026-05-14-m4-session-configuration.md) |
| M5 Fleet Management | Multi-select, bulk ops, archive/restore, tags | complete | — | [plan](docs/superpowers/plans/2026-05-12-m5-fleet-management.md) |
| M6 Polish & Onboarding | Session filter, help overlay, headless mode | complete | — | [plan](docs/superpowers/plans/2026-05-13-m6-polish-onboarding.md) |
| Auto-escalation | Poller-driven SendKeys to conductor when worker goes waiting | complete | [spec](docs/superpowers/specs/2026-05-16-auto-escalation-design.md) | [plan](docs/superpowers/plans/2026-05-16-auto-escalation.md) |
| Tool Flags | Per-session agent flags (schema v6); `claude-dangerous` preset | partial (BUG-013) | [spec](docs/superpowers/specs/2026-05-17-tool-flags-design.md) | — |
| Conductor Enhancements | Reply-to-worker routing, heartbeat, `--init-conductor-docs` | complete | [spec](docs/superpowers/specs/2026-05-18-conductor-enhancements-plan.md) | [plan](docs/superpowers/plans/2026-05-18-conductor-enhancements.md) |
| Hook Handler | Claude Code lifecycle hooks → real-time conductor updates; `install-hooks` to register | complete | [spec](docs/superpowers/specs/2026-05-23-hook-handler-design.md) | [plan](docs/superpowers/plans/2026-05-23-hook-handler.md) |
| Vim Mode Auto-Detection | Per-pane INSERT/COMMAND detection for send-pane and broadcast; `i`-only prefix; Enter submission | complete | [spec](docs/superpowers/specs/2026-05-23-vim-mode-auto-detection-design.md) | — |
| Session Worktree Options | New-session form spawns a `git worktree` per session; BRANCH field triggers worktree creation | complete | [spec](docs/superpowers/specs/2026-05-22-session-worktree-options-design.md) | [plan](docs/superpowers/plans/2026-05-23-session-worktree-options.md) |

## Known Bugs

Tracked in [docs/bugs.md](docs/bugs.md).

**Open:** BUG-013 — per-session tool flags (`ToolFlags` DB field) are stored and displayed but not passed to the agent process. A `claude-dangerous` preset was added as a workaround for `--dangerously-skip-permissions`. The general free-text flags mechanism is broken; root cause under investigation (see bug doc for details).

Key fixed bugs that shaped the architecture:
- **BUG-005** — idle detection now tracks actual pane-output changes (`lastOutput` map in poller); `DetectStatus` idle check runs before spinner heuristics.
- **BUG-010** — startup `running` flash fixed by seeding `lastChange` from tmux `#{session_activity}` / DB `last_active` instead of `now`.

<!-- tmux-agent-deck:conductor-role:start -->
## Conductor Role

You are the conductor for tmux-agent-deck worker sessions.

When you receive a message beginning with "Escalation from ...":

- Identify what the worker is blocked on.
- Use the included status, notes, and context to decide the next action.
- If more repo context is needed, inspect the local project files before answering.
- Reply with a concise unblock instruction the worker can follow.
- Prefer specific commands, file paths, tests, or implementation steps.
- Do not make broad unrelated changes.
- If the escalation lacks enough context, ask one targeted follow-up question.

When sending a reply back to a worker, use:

@deck-reply worker=<session-id>
<reply body>
@deck-end
<!-- tmux-agent-deck:conductor-role:end -->
