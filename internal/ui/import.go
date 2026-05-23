package ui

import (
	"sort"
	"strings"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	tea "github.com/charmbracelet/bubbletea"
)

type importCandidate struct {
	Name string
	Path string
}

type importState struct {
	candidates []importCandidate
	cursor     int
	selected   string
	title      string
	group      string
	formErr    string
	focus      int // 0 = title, 1 = group
}

func (m *Model) openImportPicker() {
	names, err := db.ListUntrackedTmuxSessions(m.conn, m.tmuxC)
	if err != nil {
		m.err = err
		return
	}
	sort.Strings(names)
	cands := make([]importCandidate, 0, len(names))
	for _, n := range names {
		path := ""
		if info, err := m.tmuxC.SessionInfo(n); err == nil {
			path = info.CurrentPath
		}
		cands = append(cands, importCandidate{Name: n, Path: path})
	}
	m.imp = importState{candidates: cands}
	m.mode = "import-picker"
}

func (m *Model) updateImportPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if m.imp.cursor > 0 {
			m.imp.cursor--
		}
	case tea.KeyDown:
		if m.imp.cursor < len(m.imp.candidates)-1 {
			m.imp.cursor++
		}
	case tea.KeyEsc:
		m.mode = ""
		m.imp = importState{}
	case tea.KeyEnter:
		if len(m.imp.candidates) == 0 {
			m.mode = ""
			m.imp = importState{}
			return m, nil
		}
		c := m.imp.candidates[m.imp.cursor]
		m.imp.selected = c.Name
		m.imp.title = c.Name
		m.imp.group = m.currentGroupPath()
		if m.imp.group == "" {
			m.imp.group = defaultGroupPath
		}
		m.imp.formErr = ""
		m.imp.focus = 0
		m.mode = "import-form"
	}
	return m, nil
}

func (m *Model) updateImportForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type != tea.KeyEnter {
		m.imp.formErr = ""
	}
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = ""
		m.imp = importState{}
	case tea.KeyTab:
		m.imp.focus = 1 - m.imp.focus
	case tea.KeyBackspace:
		if m.imp.focus == 0 && len(m.imp.title) > 0 {
			r := []rune(m.imp.title)
			m.imp.title = string(r[:len(r)-1])
		} else if m.imp.focus == 1 && len(m.imp.group) > 0 {
			r := []rune(m.imp.group)
			m.imp.group = string(r[:len(r)-1])
		}
	case tea.KeySpace:
		if m.imp.focus == 0 {
			m.imp.title += " "
		} else {
			m.imp.group += " "
		}
	case tea.KeyEnter:
		if _, err := db.ImportSession(m.conn, m.tmuxC, db.ImportRequest{
			TmuxName:  m.imp.selected,
			Title:     m.imp.title,
			GroupPath: m.imp.group,
		}); err != nil {
			m.imp.formErr = err.Error()
			return m, nil
		}
		m.mode = ""
		m.imp = importState{}
		if err := m.Reload(); err != nil {
			m.err = err
		}
	default:
		if len(msg.Runes) > 0 {
			r := string(msg.Runes)
			if m.imp.focus == 0 {
				m.imp.title += r
			} else {
				m.imp.group += r
			}
		}
	}
	return m, nil
}

func (m *Model) renderImportPicker() string {
	var b strings.Builder
	b.WriteString(formHeaderStyle.Render("IMPORT TMUX SESSION"))
	b.WriteString("\n\n")
	if len(m.imp.candidates) == 0 {
		b.WriteString(formHintStyle.Render("no untracked tmux sessions"))
		b.WriteString("\n\n")
		b.WriteString(formHintStyle.Render("esc: close"))
		return b.String()
	}
	for i, c := range m.imp.candidates {
		marker := "  "
		if i == m.imp.cursor {
			marker = "> "
		}
		line := marker + c.Name
		if c.Path != "" {
			line += "  " + formHintStyle.Render(c.Path)
		}
		if i == m.imp.cursor {
			line = formLabelActive.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(formHintStyle.Render("enter: select   esc: cancel"))
	return b.String()
}

func (m *Model) renderImportForm() string {
	var b strings.Builder
	b.WriteString(formHeaderStyle.Render("IMPORT: " + m.imp.selected))
	b.WriteString("\n\n")

	titleLine := "  TITLE  " + m.imp.title
	groupLine := "  GROUP  " + m.imp.group
	if m.imp.focus == 0 {
		titleLine = formLabelActive.Render("> TITLE  ") + m.imp.title + formCursorStyle.Render(" ")
	}
	if m.imp.focus == 1 {
		groupLine = formLabelActive.Render("> GROUP  ") + m.imp.group + formCursorStyle.Render(" ")
	}
	b.WriteString(titleLine + "\n")
	b.WriteString(groupLine + "\n\n")
	b.WriteString(formHintStyle.Render("tab: next   enter: import   esc: cancel"))
	if m.imp.formErr != "" {
		b.WriteString("\n")
		b.WriteString(formErrStyle.Render(m.imp.formErr))
	}
	return b.String()
}
