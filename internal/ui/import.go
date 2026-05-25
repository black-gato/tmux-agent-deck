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
	candidates  []importCandidate
	cursor      int
	selected    string
	title       string
	titleCursor int
	group       string
	groupCursor int
	formErr     string
	focus       int // 0 = title, 1 = group
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
		m.imp.titleCursor = len([]rune(c.Name))
		m.imp.group = m.currentGroupPath()
		if m.imp.group == "" {
			m.imp.group = defaultGroupPath
		}
		m.imp.groupCursor = len([]rune(m.imp.group))
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
	case tea.KeyLeft:
		if m.imp.focus == 0 {
			m.imp.titleCursor = moveCursorLeft(m.imp.titleCursor)
		} else {
			m.imp.groupCursor = moveCursorLeft(m.imp.groupCursor)
		}
	case tea.KeyRight:
		if m.imp.focus == 0 {
			m.imp.titleCursor = moveCursorRight(m.imp.title, m.imp.titleCursor)
		} else {
			m.imp.groupCursor = moveCursorRight(m.imp.group, m.imp.groupCursor)
		}
	case tea.KeyBackspace:
		if m.imp.focus == 0 {
			deleteRune(&m.imp.title, &m.imp.titleCursor)
		} else {
			deleteRune(&m.imp.group, &m.imp.groupCursor)
		}
	case tea.KeySpace:
		if m.imp.focus == 0 {
			insertRune(&m.imp.title, &m.imp.titleCursor, ' ')
		} else {
			insertRune(&m.imp.group, &m.imp.groupCursor, ' ')
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
			if m.imp.focus == 0 {
				insertRune(&m.imp.title, &m.imp.titleCursor, msg.Runes[0])
			} else {
				insertRune(&m.imp.group, &m.imp.groupCursor, msg.Runes[0])
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
		titleLine = formLabelActive.Render("> TITLE  ") + renderWithCursor(m.imp.title, m.imp.titleCursor)
	}
	if m.imp.focus == 1 {
		groupLine = formLabelActive.Render("> GROUP  ") + renderWithCursor(m.imp.group, m.imp.groupCursor)
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
