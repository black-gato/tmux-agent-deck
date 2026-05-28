package state

import (
	"strings"
	"unicode"
)

// trimPromptPrefix strips leading whitespace and known prompt-prefix glyphs
// (❯, >, ⏺) so the parser can recognize markers at the logical start of a
// line while still tolerating tmux/TUI chrome. Mid-line occurrences of the
// markers (e.g. inside an echoed escalation message) are *not* trimmed, so
// they will not be parsed as the start of a reply block.
func trimPromptPrefix(line string) string {
	for {
		trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
		switch {
		case strings.HasPrefix(trimmed, "❯"):
			line = strings.TrimPrefix(trimmed, "❯")
		case strings.HasPrefix(trimmed, ">"):
			line = strings.TrimPrefix(trimmed, ">")
		case strings.HasPrefix(trimmed, "⏺"):
			line = strings.TrimPrefix(trimmed, "⏺")
		default:
			return trimmed
		}
	}
}

type ReplyBlock struct {
	WorkerID string
	Body     string
}

const replyPrefix = "@deck-reply worker="
const endMarker = "@deck-end"

func ParseReplyBlocks(output string) []ReplyBlock {
	var results []ReplyBlock
	lines := strings.Split(output, "\n")
	i := 0
	for i < len(lines) {
		// Require @deck-reply at the logical start of the line (after stripping
		// known prompt-prefix glyphs and whitespace). This prevents mid-line
		// occurrences — such as the echoed "Reply with: @deck-reply …" inside
		// an escalation message — from being parsed as the start of a block.
		line := trimPromptPrefix(lines[i])
		if !strings.HasPrefix(line, replyPrefix) {
			i++
			continue
		}
		rest := strings.TrimSpace(line[len(replyPrefix):])
		if rest == "" {
			i++
			continue
		}
		i++

		// Single-line form: @deck-reply worker=<id> <body> @deck-end
		// Use LastIndex so the body can legitimately mention "@deck-end".
		if endIdx := strings.LastIndex(rest, endMarker); endIdx >= 0 {
			content := strings.TrimSpace(rest[:endIdx])
			workerID, body := splitWorkerBody(content)
			if workerID != "" && body != "" {
				results = append(results, ReplyBlock{WorkerID: workerID, Body: body})
			}
			continue
		}

		// Multi-line form: id alone on the @deck-reply line, body lines follow
		workerID, inlineBody := splitWorkerBody(rest)
		var bodyLines []string
		if inlineBody != "" {
			bodyLines = append(bodyLines, inlineBody)
		}
		closed := false
		for i < len(lines) {
			bl := strings.TrimSpace(lines[i])
			// Only treat @deck-end as the terminator when it appears at the
			// end of the line (or with only whitespace after) — otherwise the
			// body may legitimately mention "@deck-end" mid-sentence.
			if idx := strings.LastIndex(bl, endMarker); idx >= 0 && strings.TrimSpace(bl[idx+len(endMarker):]) == "" {
				if pre := strings.TrimSpace(bl[:idx]); pre != "" {
					bodyLines = append(bodyLines, pre)
				}
				closed = true
				i++
				break
			}
			bodyLines = append(bodyLines, bl)
			i++
		}
		if !closed {
			break
		}
		var nonEmpty []string
		for _, bl := range bodyLines {
			if bl != "" {
				nonEmpty = append(nonEmpty, bl)
			}
		}
		if len(nonEmpty) == 0 {
			continue
		}
		results = append(results, ReplyBlock{
			WorkerID: workerID,
			Body:     strings.Join(nonEmpty, " | "),
		})
	}
	return results
}

// splitWorkerBody splits "id body text" at the first space.
func splitWorkerBody(s string) (workerID, body string) {
	idx := strings.Index(s, " ")
	if idx < 0 {
		return s, ""
	}
	return s[:idx], strings.TrimSpace(s[idx+1:])
}

func NewOutputSince(baseline, current string) string {
	idx := strings.LastIndex(current, baseline)
	if idx < 0 {
		return current
	}
	return current[idx+len(baseline):]
}
