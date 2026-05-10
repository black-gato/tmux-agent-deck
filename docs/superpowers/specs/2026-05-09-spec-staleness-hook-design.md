# Spec Staleness Hook Design

**Date:** 2026-05-09
**Status:** Approved

## Overview

A two-part hook system that checks whether design specs are still accurate after code changes. The Claude Code PostToolUse hook injects findings into the active session so divergences are caught mid-development. The git pre-commit hook acts as a hard gate, blocking commits when staged code contradicts a spec.

---

## Components

```
.spec-map.json                   ← glob → spec file mapping (project root)
.claude/
  settings.json                  ← registers PostToolUse hook
  hooks/
    spec-check.sh                ← shared checker (two modes: --hook, --precommit)
.git/hooks/
  pre-commit                     ← thin wrapper: calls spec-check.sh --precommit
```

---

## Spec Mapping

`.spec-map.json` at the project root maps file globs to one or more spec files. More specific glob entries take precedence over broader ones.

```json
{
  "internal/tmux/client.go": [
    "docs/superpowers/specs/2026-05-09-return-to-tui-design.md"
  ],
  "internal/**": [
    "docs/superpowers/specs/2026-05-09-tmux-agent-deck-design.md"
  ],
  "cmd/**": [
    "docs/superpowers/specs/2026-05-09-tmux-agent-deck-design.md"
  ]
}
```

A file can match multiple entries — all matched specs are checked and deduplicated before API calls. New specs are registered here as the project grows.

---

## Hook Behavior

### Claude Code mode (`--hook`)

Triggered by the `PostToolUse` event for `Edit` and `Write` tool calls.

1. Parse stdin JSON — extract `tool_input.file_path`
2. Look up matched spec files in `.spec-map.json` (glob match against `file_path`)
3. For each matched spec: read spec file + code file, call API, collect divergences
4. If divergences found: print formatted report to stdout (Claude sees it as a system message)
5. If no spec matches, exit 0 silently
6. If `ANTHROPIC_API_KEY` is unset, print a one-line warning to stderr and exit 0

### Git pre-commit mode (`--precommit`)

1. Get list of staged `.go` files via `git diff --cached --name-only --diff-filter=ACM`
2. Deduplicate spec files matched across all staged files
3. For each unique (code file, spec file) pair: call API, collect divergences
4. If any divergences found: print full report and exit 1 (blocks commit)
5. If no staged files match any spec, exit 0 silently
6. If `ANTHROPIC_API_KEY` is unset, print a warning and exit 0 (never block on missing key)

---

## API Call

**Endpoint:** `POST https://api.anthropic.com/v1/messages`  
**Model:** `claude-haiku-4-5-20251001` (fast, low cost for a lint-style check)  
**Auth:** `x-api-key: $ANTHROPIC_API_KEY`

**System prompt:**
```
You are a spec compliance checker. Given a design spec and a code file, list any divergences where the code contradicts or is missing something the spec explicitly requires. Be concise — one line per divergence. If fully compliant, respond with exactly: COMPLIANT
```

**User message:**
```
Spec file: <spec_path>
<spec content>

Code file: <file_path>
<code content>
```

---

## Error Handling

| Condition | Behavior |
|-----------|----------|
| `ANTHROPIC_API_KEY` unset | Warn to stderr, exit 0 (never block) |
| API call fails (non-200) | Warn to stderr with status code, exit 0 |
| `jq` not installed | Warn to stderr, exit 0 |
| Spec file missing | Skip that spec, continue |
| Code file missing | Skip that pair, continue |
| API returns `COMPLIANT` | Exit 0 silently |

Errors in the hook never block the operation — the check is advisory in CC mode and a soft gate in pre-commit mode (key missing = pass).

---

## Hook Registration

`.claude/settings.json`:
```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": ".claude/hooks/spec-check.sh --hook"
          }
        ]
      }
    ]
  }
}
```

The `.git/hooks/pre-commit` script is not committed to the repo (git ignores `.git/`). The implementation plan includes a setup script or instructions for installing it.

---

## Out of Scope

- Checking spec files themselves for internal consistency
- Multi-file diffs (each file is checked independently against its spec)
- Configurable model or token limits
- Caching API responses
