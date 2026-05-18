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
		workerID := strings.TrimSpace(strings.TrimPrefix(line, "@deck-reply worker="))
		if workerID == "" {
			i++
			continue
		}
		i++
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

func NewOutputSince(baseline, current string) string {
	idx := strings.LastIndex(current, baseline)
	if idx < 0 {
		return current
	}
	return current[idx+len(baseline):]
}
