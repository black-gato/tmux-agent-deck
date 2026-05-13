# tmux-agent-deck

A terminal UI for managing multiple AI coding agent sessions in tmux, organized into nested groups.

## Overview

tmux-agent-deck lets you run many AI coding agents (Claude, Gemini, etc.) in parallel tmux sessions and monitor them all from a single interface. Sessions are grouped hierarchically, and their status is detected automatically by reading tmux pane output in the background.

```
┌─ tmux-agent-deck ──────────────────────────────┐
│                                                  │
│  ▼ work                                          │
│    ▼ frontend                                    │
│      ● my-app          claude   running          │
│      ○ api-refactor    claude   waiting          │
│    ► backend           (collapsed)               │
│                                                  │
│  ▼ personal                                      │
│      ◐ side-project    gemini   idle             │
│                                                  │
│  ▼ my-sessions                                   │
│      ✕ old-bug-fix     claude   error            │
│                                                  │
│ [n]ew  [g]roup  [m]ove  [r]ename  [d]elete  [q]uit │
└──────────────────────────────────────────────────┘
```

## Status Indicators

| Symbol | Meaning |
|--------|---------|
| `●` | Running / thinking |
| `○` | Waiting for input |
| `◐` | Idle (no activity 30s+) |
| `✕` | Error / process dead |
| `—` | Stopped |

## Installation

```bash
go install github.com/black-gato/tmux-agent-deck@latest
```

Or build from source:

```bash
git clone https://github.com/black-gato/tmux-agent-deck
cd tmux-agent-deck
go build -o tmux-agent-deck .
```

**Requirements:** Go 1.22+, tmux

## Usage

### Launch the TUI

```bash
tmux-agent-deck
```

### Run headless monitoring

```bash
tmux-agent-deck --headless --notify --notify-style digest --poll 2s
```

### CLI Commands

```bash
# Sessions
tmux-agent-deck add --title "my-app" --project /path/to/project --group work
tmux-agent-deck list [--json]
tmux-agent-deck remove <id|title>
tmux-agent-deck session start <id|title>
tmux-agent-deck session stop <id|title>
tmux-agent-deck session attach <id|title>

# Groups
tmux-agent-deck group create <path>            # e.g. work/frontend
tmux-agent-deck group delete <path>
tmux-agent-deck group move <session> <group>
```

### Global Flags

```bash
tmux-agent-deck --notify
tmux-agent-deck --notify-style waiting|conductor|digest
tmux-agent-deck --notify-quiet cooldown=10m,hours=22:00-07:00
tmux-agent-deck --poll 500ms
tmux-agent-deck --headless
```

### TUI Keybindings

| Key | Action |
|-----|--------|
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `Enter` | Attach to session |
| `Space` | Collapse / expand group or select session |
| `n` | New session in current group |
| `g` | New group |
| `m` | Move session to group |
| `r` | Rename session or group |
| `d` | Delete session or group |
| `a` / `A` | Archive session / toggle archived view |
| `x` | Send keys to a session or selection |
| `f` | Fork session |
| `b` | Broadcast to a group |
| `e` | Edit notes |
| `t` | Edit tags |
| `c` / `C` | Set conductor / escalate to conductor |
| `v` | Toggle full output view |
| `/` | Filter session list |
| `?` | Open keyboard shortcut help |
| `q` | Quit |

## How It Works

- State is stored in SQLite at `~/.tmux-agent-deck/state.db`
- A background poller reads `tmux capture-pane` output on a configurable interval (`--poll`) to detect session status
- The TUI reloads from the database on each tick and re-renders
- `--headless` runs the same poller and notification pipeline without launching the TUI
- Sessions inherit the default tool from their group at creation time

## Tech Stack

- [Cobra](https://github.com/spf13/cobra) — CLI
- [Bubbletea](https://github.com/charmbracelet/bubbletea) + [Lipgloss](https://github.com/charmbracelet/lipgloss) — TUI
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) — pure Go SQLite (no CGO)

## License

MIT — see [LICENSE](LICENSE)
