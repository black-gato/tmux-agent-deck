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
│   │   ├── poller.go    # Poller: Start/Stop/PollOnce, TmuxReader interface
│   │   └── poller_test.go
│   ├── ui/
│   │   ├── app.go       # Bubbletea Model, Init/Update/View, Reload()
│   │   ├── app_test.go
│   │   ├── list.go      # ListItem, BuildTree(), RenderList()
│   │   ├── list_test.go
│   │   ├── dialog.go    # dialogState, updateDialog(), commitDialog()
│   │   └── keys.go      # actionForKey() mapping
│   └── testutil/
│       ├── db.go        # OpenTestDB(t) helper
│       └── tmux.go      # FakeTmuxClient for tests
```

**Data flow:** `poller` reads tmux pane output every ~1s → writes status to DB → `app` reads DB on tick → bubbletea re-renders.

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
Sequential version checks in `migrate()` using a `metadata` table (`key=schema_version`). Current version: 1. WAL/busy_timeout pragmas run on every open before migration.

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

## Running Tests

```bash
go test ./...
```

## Building

```bash
go build -o tmux-agent-deck .
```

## In-Progress Features

See `docs/superpowers/specs/` for approved designs and `docs/superpowers/plans/` for implementation plans.

**Split panel TUI** (`docs/superpowers/specs/2026-05-09-split-panel-tui-design.md`, plan: `docs/superpowers/plans/2026-05-10-split-panel-tui.md`) — 6 of 9 tasks remain. Adds a persistent 35/65 split layout with a detail panel showing session name, group, pane programs, live output, and inline-editable notes.

## Roadmap

See `docs/superpowers/specs/2026-05-10-roadmap.md` for the full product roadmap.

After the split panel TUI, planned milestones in order:
1. **M1 Interaction Primitives** — Send to pane (`x`), Fork session (`f`), Broadcast to group (`b`)
2. **M2 Observability** — Context window %, waiting timers, desktop notifications, output search
3. **M3 Fleet Management** — Multi-select, bulk ops, archive/restore, tags
4. **M4 Session Configuration** — Project path picker, tool selection, group defaults, startup scripts
5. **M5 Polish** — Session filter, help overlay, onboarding, configurable poll, headless mode
