# Conductor Enhancements Plan

**Date:** 2026-05-18
**Status:** Planned

## Overview

Conductor mode currently routes worker escalations to a designated conductor session, but the workflow is one-way: the app sends an escalation to the conductor and does not automatically return the conductor's answer to the blocked worker. This plan extends conductor mode in three parts:

- Reply-to-worker routing from conductor responses
- Tunable conductor heartbeat checks for worker groups
- Optional conductor role documentation in `CLAUDE.md`

The goal is to let a conductor supervise worker sessions with less manual relay while keeping the workflow explicit, inspectable, and opt-in.

## Current Behavior

- `c` designates a conductor for the selected group.
- `C` manually escalates the selected session to that group's conductor.
- `--auto-escalate` sends an escalation to the conductor when a worker transitions to `waiting`.
- Escalation messages are sent with `SendKeys` and submitted with `SendRawKeys(..., "Enter")`.
- The app does not watch the conductor pane for responses.
- The app does not automatically send conductor answers back to workers.

Relevant files:

- `internal/state/poller.go`
- `internal/state/escalate.go`
- `internal/ui/app.go`
- `internal/tmux/client.go`

## Feature 1: Reply-To-Worker Workflow

### Product Behavior

When `--auto-escalate` is enabled, the conductor can reply to an escalation using an explicit block marker. The poller detects the completed reply block and sends the reply back to the original worker session.

Conductor reply format:

```text
@deck-reply worker=<session-id>
<reply body>
@deck-end
```

Worker receives:

```text
Conductor reply: <reply body>
```

Multiline replies are normalized into one prompt-safe line by trimming non-empty lines and joining them with ` | `.

### Escalation Message Changes

Auto and manual escalation messages should include:

- Worker title
- Worker DB session ID
- Status
- Notes, when present
- Filtered context
- Compact reply instructions

Example:

```text
Escalation from worker-a
Worker ID: 12
Status: waiting
Reply with: @deck-reply worker=12 ... @deck-end
Current issue context:
...
```

The exact sent tmux message can remain single-line normalized, matching the current escalation delivery style.

### Poller Flow

During `PollOnce`, after normal status updates and auto-escalation:

1. Find active conductor sessions for groups.
2. Capture conductor pane output.
3. On first observation of each conductor pane, establish a baseline and ignore historical output.
4. Parse complete `@deck-reply` blocks only from new output.
5. Resolve the worker with `db.GetSession(workerID)`.
6. Skip unknown workers and workers in `stopped` or `error`.
7. Send `Conductor reply: <body>` to the worker pane.
8. Submit with `SendRawKeys(..., "Enter")`.
9. Track processed reply fingerprints in memory so visible markers are not resent every poll.

### State Model

Use in-memory state only for v1:

- Conductor pane baseline per conductor session
- Processed reply fingerprints

No database migration is needed. If the app restarts, old visible reply blocks are ignored by the initial baseline pass.

### Enablement

Reply scanning is active only when:

- `--auto-escalate` is enabled
- The poller has a configured tmux sender

Manual `C` escalations can include reply instructions, but automatic reply scanning still depends on `--auto-escalate`.

### Tests

Parser tests:

- Complete block parses worker ID and body
- Incomplete block is ignored
- Multiple blocks parse independently
- Empty body is ignored
- Body lines are trimmed and normalized

Poller tests:

- Baseline ignores old conductor markers
- New marker sends one reply to the worker
- Duplicate visible marker is not resent
- Unknown worker ID is skipped
- Stopped/error worker is skipped
- Only conductor panes are scanned

Escalation tests:

- Escalation message includes worker ID
- Escalation message includes reply syntax

## Feature 2: Tunable Conductor Heartbeat

### Product Behavior

Add a heartbeat that periodically prompts each conductor with a digest of worker state for the conductor's group. This lets the conductor actively check blocked work even when no new `waiting` transition occurs.

CLI flag:

```text
--conductor-heartbeat <duration>
```

Default:

```text
0s
```

`0s` disables the heartbeat.

### Enablement

Heartbeat is active only when:

- `--auto-escalate` is enabled
- `--conductor-heartbeat` is greater than `0s`
- The group has a conductor

### Group Scope

For each conductor-assigned group:

- Include sessions in that group.
- Include inherited child groups.
- Exclude child groups that define their own conductor.

### Digest Behavior

On each heartbeat interval, send a conductor-facing digest:

- Group path
- Waiting worker count
- Waiting worker titles, IDs, and waiting duration
- Current status summary for the rest of the group

If no workers are waiting, send an "All clear" heartbeat so the conductor knows the app is still monitoring.

### Implementation Notes

Add to `Poller`:

```go
func (p *Poller) SetConductorHeartbeat(interval time.Duration)
```

Track last heartbeat time per conductor group in memory. Run the heartbeat check after the normal `PollOnce` session scan so it uses fresh statuses.

No database migration is needed.

## Feature 3: Conductor `CLAUDE.md` Initialization

### Product Behavior

When the app is launched with an opt-in flag, assigning a conductor can also initialize conductor instructions in the project's `CLAUDE.md`.

CLI flag:

```text
--init-conductor-docs
```

Default:

```text
false
```

Trigger:

- Pressing `c` to set a conductor

Target:

- The selected conductor session's `ProjectPath`
- `CLAUDE.md` only

### Managed Block

Use a managed section so the app can update its own conductor instructions without overwriting user content:

```md
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
```

### File Rules

- If `CLAUDE.md` is missing, create it.
- If the managed block exists, replace only that block.
- If `CLAUDE.md` has user content but no managed block, append the managed block.
- If the file has an unmanaged `## Conductor Role`, preserve it and append the managed block separately.
- Do not create `Conductor.md`, `Conducter.md`, or `.agent-policy.md` in v1.

## Suggested Sequencing

1. Implement reply parser as pure functions.
2. Add worker ID and reply syntax to escalation messages.
3. Add conductor pane baseline and reply scanning to the poller.
4. Add reply delivery tests.
5. Add `--conductor-heartbeat` after reply routing is stable.
6. Add `--init-conductor-docs` last because it touches project files outside the app database.

## Out Of Scope For V1

- Persistent reply queue
- Webhook delivery
- Multi-conductor fan-out
- Rich terminal UI for reply history
- Automatically inferring conductor replies without explicit markers
- Creating policy files beyond `CLAUDE.md`
