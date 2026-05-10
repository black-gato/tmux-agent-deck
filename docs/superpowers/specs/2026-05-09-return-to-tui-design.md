# Return to TUI Keybinding Design

**Date:** 2026-05-09  
**Status:** Approved

## Overview

When a user attaches to an agent session from the TUI, they are dropped into a tmux session with no obvious way back. This feature adds a `prefix + q` keybinding that detaches the client and returns to the plain terminal where the TUI is running.

---

## Behavior

1. User presses `Enter` on a session in the TUI → `Attach()` is called
2. Before attaching, the existing `prefix + q` binding (if any) is saved
3. `prefix + q` is set to `detach-client`
4. `tmux attach-session -t <session>` blocks until the user detaches
5. On return (any exit), the original `prefix + q` binding is restored

The TUI process is still running while the user is in the tmux session; detaching returns them to it automatically.

---

## Implementation

**File:** `internal/tmux/client.go`

All changes are confined to the `Attach()` method. No DB, TUI, CLI, or schema changes.

### Flow

```
saveBinding("q")           → tmux list-keys -T prefix q
setBinding("q", "detach-client")   → tmux bind-key -T prefix q detach-client
defer restoreBinding("q", saved)   → runs on any return from Attach()
tmux attach-session -t <session>   → blocks
```

### Parsing `list-keys` output

Output format: `bind-key -T prefix q <command>`

Strip the `bind-key -T prefix q ` prefix; keep the rest as the restore command. If `list-keys` returns nothing or exits non-zero, there was no existing binding — restore is `tmux unbind-key -T prefix q`.

### Error handling

If `bind-key` or `unbind-key` calls fail, log the error but do not block the attach. The core operation (attaching to the session) always proceeds.

### Key

Hardcoded to `q` for this implementation. Can be made configurable later via a field on the `Client` struct.

---

## Out of Scope

- Configurable key (post-MVP)
- Support for launching the TUI inside tmux (separate concern)
