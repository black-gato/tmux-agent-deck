package state

import "strings"

type ReplyBlock struct {
	WorkerID string
	Body     string
}

func ParseReplyBlocks(output string) []ReplyBlock {
	var results []ReplyBlock
	lines := strings.Split(output, "\n")
	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "@deck-reply worker=") {
			i++
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "@deck-reply worker="))
		if rest == "" {
			i++
			continue
		}
		i++

		// Single-line form: @deck-reply worker=<id> <body> @deck-end
		if endIdx := strings.Index(rest, "@deck-end"); endIdx >= 0 {
			content := strings.TrimSpace(rest[:endIdx])
			workerID, body := splitWorkerBody(content)
			if workerID != "" && body != "" {
				results = append(results, ReplyBlock{WorkerID: workerID, Body: body})
			}
			continue
		}

		// Multi-line form: id alone on the @deck-reply line, body lines follow
		workerID := rest
		var bodyLines []string
		closed := false
		for i < len(lines) {
			bl := strings.TrimSpace(lines[i])
			if bl == "@deck-end" {
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
