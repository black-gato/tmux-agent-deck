# Known Bugs

Tracked bugs in tmux-agent-deck. Newest first. Status: `open`, `in-progress`, `fixed`.

Current repo status as of 2026-05-14: BUG-001 through BUG-004 are fixed in the current branch. Verified by inspecting the implementation and running `go test ./...`.

---

## BUG-005: idle detection uses stale output heuristics, so inactive sessions can remain `running`

**Reported:** 2026-05-14
**Status:** open
**Severity:** medium (misleading status; weakens observability and waiting triage)

### Symptom

A tmux session can sit untouched for well over 30 seconds and still display as `running` instead of `idle`.

Common example:

1. The pane output contains text like `Thinking...`, `Running tests...`, or an old spinner glyph.
2. The underlying process stops producing new output.
3. Even after 30+ seconds, the session remains `running`.

### Root cause

Status detection in [internal/tmux/status.go](/Users/anthonymirville/Projects/tmux-agent-deck/.worktrees/feature-feedback/internal/tmux/status.go:56) is heuristic:

1. If the current pane tail contains spinner characters or the substrings `Thinking` / `Running`, it returns `running` immediately.
2. The idle fallback only triggers when none of those markers are present and `time.Since(lastChange) > 30*time.Second`.

But `lastChange` in [internal/state/poller.go](/Users/anthonymirville/Projects/tmux-agent-deck/.worktrees/feature-feedback/internal/state/poller.go:145) is not the time of the last pane-output change. It is only updated when the derived status changes:

```go
if newStatus != s.Status {
    p.setLastChange(s.ID, now)
    ...
}
```

As a result, stale pane text that still looks "running-ish" can pin the session in `running` indefinitely, and the 30-second rule is not a true inactivity timer.

### Planned fix

Track actual pane-output changes between polls and base idle detection on that signal instead of stale substrings alone.

1. **`internal/state/poller.go`** — store the previous captured pane output (or a compact hash of it) per session.
2. On each poll, compare the newly captured output to the previous value.
3. Update a `lastOutputChange` timestamp only when the pane contents actually change.
4. Pass that timestamp into status detection instead of the current status-transition timestamp.
5. Keep prompt detection for `waiting`, but treat stale `Thinking` / `Running` text as historical output unless the pane is still changing.

### Test plan

- `internal/state/poller_test.go` — when pane output stops changing for more than 30 seconds, status becomes `idle` even if the tail still contains `Thinking` or `Running`.
- `internal/state/poller_test.go` — when pane output continues changing between polls, status stays `running`.
- `internal/tmux/status_test.go` — narrow status detection to prompt/running-marker interpretation only, without relying on stale text to suppress idle forever.

### Files touched

- `internal/state/poller.go`
- `internal/state/poller_test.go`
- `internal/tmux/status.go`
- `internal/tmux/status_test.go`

---

## BUG-004: pressing `C` on a non-waiting session surfaces an error instead of no-op

**Reported:** 2026-05-13
**Status:** fixed
**Severity:** low (jarring UX; no data loss)

### Symptom

Pressing `C` (escalate to conductor) on any session that is not in the `waiting` state produces:

```
error: session "hell" is not waiting
```

This strands the user on the error screen (compounded by BUG-001).

### Root cause

`internal/ui/app.go:742` in `escalateSelectedSession`:

```go
if session.Status != tmux.StatusWaiting {
    return fmt.Errorf("session %q is not waiting", session.Title)
}
```

The escalate action only makes sense for a waiting session, but instead of silently doing nothing (the pattern used by other guarded actions), it returns an error that propagates to `m.err`.

### Resolution

`internal/ui/app.go` now uses the guarded no-op:

```go
if session.Status != tmux.StatusWaiting {
    return nil
}
```

The non-waiting path no longer surfaces an error. Escalation still errors for genuinely invalid states such as "already the conductor" or "conductor not running".

### Test plan

- `internal/ui/app_test.go` — pressing `C` on a running/idle/stopped session sets neither `m.err` nor `m.mode`; model state is unchanged.
- Existing escalation tests continue to pass for the waiting-session path.

### Files touched

- `internal/ui/app.go`
- `internal/ui/app_test.go`

---

## BUG-003: send-pane and broadcast send Enter after text; `-l` flag missing from send-keys

**Reported:** 2026-05-13
**Status:** fixed
**Severity:** medium (send behavior is unreliable; Enter fires commands unintentionally)

### Symptom

Using `x` (send-pane) or `b` (broadcast), the text typed in the dialog is submitted to the target session with a trailing Enter — the AI agent executes the command immediately rather than just seeing text typed into the prompt.

### Root cause

`internal/tmux/client.go:114` calls:

```go
runCmd("tmux", "send-keys", "-t", paneTarget(session, paneIndex), keys)
```

Two problems:

1. **Missing `-l` flag.** Without `-l`, `tmux send-keys` interprets the argument as tmux key names, not literal text. For a multi-character string that isn't a known key name, tmux falls back to character-by-character dispatch — but the behavior is version-dependent and can produce unexpected results, including spurious Enter keypresses on some tmux versions.

2. **`interceptCtrl` intentionally exploits no-`-l` mode.** `internal/ui/dialog.go:26-40` encodes Ctrl keys as strings like `"C-c"`, `"C-d"`, `"C-z"`, `"C-l"`, `"C-u"` and appends them to `m.dialog.value`. These strings are only interpreted as control characters by `tmux send-keys` when the `-l` flag is absent. Adding `-l` would fix the spurious-Enter problem but break control-key forwarding.

The two concerns are currently conflated in a single `dialog.value` string and a single `SendKeys` call, which makes it impossible to fix one without breaking the other.

### Reproduction

1. Start a running tmux session with a Claude Code agent at its `>` prompt.
2. Press `x`, type any text (e.g. `hello`), press Enter to confirm.
3. Observe that the agent's session receives `hello` AND executes it (Enter was sent).

### Resolution

The shipped fix separates literal text from control-key sequences at the send layer:

1. **`internal/tmux/client.go`** sends literal text with `tmux send-keys -l`.
2. **`internal/ui/dialog.go`** stores intercepted ctrl sequences separately in `dialogState.ctrlKeys`.
3. **`internal/tmux/client.go` / `internal/testutil/tmux.go`** use `SendRawKeys` for tmux key names such as `C-c`.

Enter is no longer appended implicitly. Submission now only happens when the user explicitly sends a control key for it.

### Test plan

- `internal/tmux` — `SendKeys` with literal text containing a space sends the text without Enter; a ctrl key `"C-c"` is sent as Ctrl+C.
- `internal/ui/app_test.go` — in send-pane dialog, `interceptCtrl` stores ctrl keys separately and committed sends split literal text from raw ctrl sequences.
- Regression: broadcast ctrl key test (sending `C-c` to broadcast still works).

### Files touched

- `internal/tmux/client.go`
- `internal/tmux/client_test.go` (or add)
- `internal/testutil/tmux.go`
- `internal/ui/dialog.go`
- `internal/ui/app_test.go`

---

## BUG-002: duplicate group path surfaces raw SQLite UNIQUE constraint error

**Reported:** 2026-05-13
**Status:** fixed
**Severity:** medium (poor UX; combines with BUG-001 to trap the user on an unreadable error screen)

### Symptom

Creating a group with a path that already exists produces:

```
constraint failed: UNIQUE constraint failed: groups.path (1555)
```

In the TUI this is rendered raw via `m.err` and (because of BUG-001) leaves the user on a dead-end error screen. In the CLI it's wrapped as `create group: <raw>`.

### Reproduction

TUI:
1. Press `g`, type `work`, Enter — group created.
2. Press `g`, type `work` again, Enter — raw SQLite error appears.

CLI:
```
tmux-agent-deck group create work
tmux-agent-deck group create work   # → error
```

### Root cause

`internal/db/groups.go:19-30` `CreateGroup` does a plain `INSERT INTO groups (path, ...)` and returns whatever SQLite emits. `path` is the primary key, so a second insert with the same path raises `SQLITE_CONSTRAINT_PRIMARYKEY` (1555). No caller translates this into a friendly message.

Same shape of issue could surface for any other `INSERT` against a unique/PK column (sessions: `id` UUID — collision astronomically unlikely, so not a practical concern; session titles are *not* unique).

Note: the bug title in the original report said "same name" but the actual constraint is on `path`. `name` is not unique. Two groups under different parents *can* share a leaf name (`work/frontend` and `personal/frontend` are fine); the collision happens when the full path matches.

### Resolution

The current implementation translates the constraint violation into a typed error at the data layer and avoids exposing raw SQL text:

1. **`internal/db/groups.go`** returns `ErrGroupExists` for duplicate `groups.path`.
2. **`cmd/group.go`** formats that as `group "X" already exists`.
3. **`internal/ui/dialog.go`** treats a duplicate create as "navigate to the existing group" instead of surfacing a raw SQLite error.

### Test plan

- `internal/db/groups_test.go` — creating a group with an existing path returns an error that satisfies `errors.Is(err, db.ErrGroupExists)`.
- `cmd/cmd_test.go` — `group create work` twice via `RunWith`; second run's stderr/stdout contains `already exists` and not `UNIQUE constraint`.
- `internal/ui/app_test.go` — duplicate group creation leaves the TUI focused on the existing group instead of creating a second entry.

### Files touched

- `internal/db/groups.go`
- `internal/db/groups_test.go`
- `cmd/group.go`
- `cmd/cmd_test.go`
- `internal/ui/dialog.go`
- `internal/ui/app_test.go`

---

## BUG-001: ctrl+c does not quit the TUI; error screen is a dead end

**Reported:** 2026-05-13
**Status:** fixed
**Severity:** medium (UX dead end; `q` still works in navigation mode)

### Symptom

When an error arises in the TUI, the error screen appears stuck — ctrl+c does nothing. The user has no visible way to quit.

### Reproduction

1. Trigger any code path that sets `m.err` (e.g. a DB failure during `Reload`, or an error from `commitDialog` that leaves the user in navigation mode).
2. The view renders only `error: <message>` with no footer or hint.
3. Press ctrl+c — nothing happens.
4. `q` does still quit (from navigation mode) but the user has no way to know that.

### Root cause

Two compounding problems:

1. **`internal/ui/keys.go` `keyTypeMap`** does not map `tea.KeyCtrlC` to any action. `actionForKey` returns `""` for ctrl+c, so `updateNavigation`'s switch falls through and the keypress is silently dropped.
2. `tea.WithAltScreen()` (`cmd/root.go:130`) puts the terminal in raw mode, so ctrl+c is delivered as a `KeyCtrlC` byte, **not** SIGINT. The parent `signal.NotifyContext` in `Execute()` and bubbletea's signal handler never see it.
3. **`internal/ui/app.go` `View()`** returns only `"error: " + m.err.Error()` when `m.err != nil` — no footer, no exit instructions. The user sees a bare error and instinctively reaches for ctrl+c.
4. In non-send dialogs (`new-session`, `rename`, `move`, `edit-notes`, `edit-tags`, `fork-session`, `new-group`, `filter`), ctrl+c is also silently dropped — only `Esc` cancels them. `send-pane` and `broadcast` intentionally intercept ctrl+c into the dialog buffer (`internal/ui/dialog.go:26-40`); that behavior is correct and must be preserved so users can send `^C` to tmux panes.

### Resolution

The current implementation applies the scoped fix:

1. **`internal/ui/keys.go`** — add `tea.KeyCtrlC: "quit"` to `keyTypeMap`. Makes ctrl+c quit globally from navigation mode.
2. **`internal/ui/dialog.go` `updateDialog`** — for dialog modes *other than* `send-pane` and `broadcast`, treat `tea.KeyCtrlC` as cancel (equivalent to `Esc`: clear `m.mode`, do not commit). The existing `interceptCtrl` path in `send-pane`/`broadcast` runs first and is unaffected.
3. **`internal/ui/app.go` `View()`** — when `m.err != nil`, append `"\n\nPress q or ctrl+c to quit"` so the error screen is no longer a dead end.

### Test plan

- Failing test: in navigation mode, send `tea.KeyMsg{Type: tea.KeyCtrlC}` and assert the returned command is `tea.Quit`.
- Failing test: in `new-session` dialog, send ctrl+c and assert `m.mode == ""` and dialog state is cleared, with no session created.
- Regression test: in `send-pane` dialog, send ctrl+c and assert the dialog buffer contains the literal `"C-c"` (existing intercept preserved).
- Failing test: when `m.err != nil`, `View()` output contains the substring `"Press q or ctrl+c to quit"`.

### Files touched

- `internal/ui/keys.go`
- `internal/ui/dialog.go`
- `internal/ui/app.go`
- `internal/ui/app_test.go`
- `internal/ui/dialog_test.go` (new or extend existing)
