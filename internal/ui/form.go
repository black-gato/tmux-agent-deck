package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
)

type fieldKind int

const (
	fieldText   fieldKind = iota
	fieldSelect
)

type formField struct {
	label   string
	kind    fieldKind
	value   string
	cursor  int      // rune index
	options []string
	optIdx  int
}

type formState struct {
	fields             []formField
	focusField         int
	candidates         []string
	candIdx            int
	candActive         bool
	candBase           string // dir prefix stable during cycling
	formErr            string
	worktreeUserEdited bool
}

var (
	formBarStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#cba6f7"))
	formLabelActive = lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa")).Bold(true)
	formLabelDim    = lipgloss.NewStyle().Faint(true)
	formValueDim    = lipgloss.NewStyle().Faint(true)
	formCursorStyle = lipgloss.NewStyle().Reverse(true)
	formHeaderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cba6f7")).Bold(true)
	formHintStyle   = lipgloss.NewStyle().Faint(true)
	formCandStyle   = lipgloss.NewStyle().Faint(true)
	formCandHLStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1"))
	formErrStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8"))
)

func insertRune(f *formField, r rune) {
	runes := []rune(f.value)
	runes = append(runes[:f.cursor], append([]rune{r}, runes[f.cursor:]...)...)
	f.value = string(runes)
	f.cursor++
}

func deleteRune(f *formField) {
	if f.cursor == 0 {
		return
	}
	runes := []rune(f.value)
	runes = append(runes[:f.cursor-1], runes[f.cursor:]...)
	f.value = string(runes)
	f.cursor--
}

func moveCursorLeft(f *formField) {
	if f.cursor > 0 {
		f.cursor--
	}
}

func moveCursorRight(f *formField) {
	if f.cursor < len([]rune(f.value)) {
		f.cursor++
	}
}

func renderWithCursor(value string, cursor int) string {
	runes := []rune(value)
	if cursor >= len(runes) {
		return value + formCursorStyle.Render(" ")
	}
	before := string(runes[:cursor])
	at := string(runes[cursor : cursor+1])
	after := string(runes[cursor+1:])
	return before + formCursorStyle.Render(at) + after
}

func (m *Model) initSessionForm() {
	defaultPath := "."
	if wd, err := os.Getwd(); err == nil {
		defaultPath = wd
	}
	defaultTool := "claude"
	cursorGroupPath := m.currentGroupPath()
	for _, g := range m.groups {
		if g.Path == cursorGroupPath {
			if g.DefaultPath != "" {
				defaultPath = g.DefaultPath
			}
			if g.DefaultTool != "" {
				defaultTool = g.DefaultTool
			}
			break
		}
	}
	toolOptions := []string{"claude", "claude-dangerous", "aider", "cursor", "bash", "custom"}
	toolIdx := 0
	for i, t := range toolOptions {
		if t == defaultTool {
			toolIdx = i
			break
		}
	}
	m.form = formState{
		fields: []formField{
			{label: "TITLE", kind: fieldText},
			{label: "PATH", kind: fieldText, value: defaultPath, cursor: len([]rune(defaultPath))},
			{label: "BRANCH", kind: fieldText},
			{label: "BASE", kind: fieldText},
			{label: "WORKTREE", kind: fieldText},
			{label: "TOOL", kind: fieldSelect, options: toolOptions, optIdx: toolIdx},
			{label: "FLAGS", kind: fieldText},
			{label: "SCRIPT", kind: fieldText},
		},
	}
}

func (m *Model) cyclePathCompletion() {
	f := &m.form.fields[1]
	if !m.form.candActive {
		dir, _ := splitPathInput(f.value)
		_, candidates := completePath(f.value)
		if len(candidates) == 0 {
			m.form.focusField = 2
			return
		}
		m.form.candidates = candidates
		m.form.candBase = dir
		m.form.candIdx = 0
		m.form.candActive = true
		f.value = dir + candidates[0]
		f.cursor = len([]rune(f.value))
		return
	}
	m.form.candIdx++
	if m.form.candIdx >= len(m.form.candidates) {
		m.resetPathCompletion()
		m.form.focusField = 2
		return
	}
	f.value = m.form.candBase + m.form.candidates[m.form.candIdx]
	f.cursor = len([]rune(f.value))
}

func (m *Model) resetPathCompletion() {
	m.form.candActive = false
	m.form.candidates = nil
	m.form.candIdx = 0
	m.form.candBase = ""
}

func slugBranch(branch string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(branch) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return b.String()
}

func DeriveWorktreePath(repo, branch string) string {
	return filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-"+slugBranch(branch))
}

func ResolveDefaultBranch(repo string) string {
	out, err := exec.Command("git", "-C", repo, "symbolic-ref", "--short", "refs/remotes/origin/HEAD").Output()
	if err == nil {
		s := strings.TrimSpace(string(out))
		if idx := strings.LastIndex(s, "/"); idx >= 0 {
			return s[idx+1:]
		}
		return s
	}
	out, err = exec.Command("git", "-C", repo, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func runGitWorktreeAdd(repo, branch, dir, base string) error {
	cmd := exec.Command("git", "-C", repo, "worktree", "add", "-b", branch, dir, base)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (m *Model) updateWorktreeDefault() {
	if m.form.worktreeUserEdited {
		return
	}
	branch := m.form.fields[2].value
	if branch == "" {
		m.form.fields[4].value = ""
		m.form.fields[4].cursor = 0
		return
	}
	path := m.form.fields[1].value
	derived := DeriveWorktreePath(path, branch)
	m.form.fields[4].value = derived
	m.form.fields[4].cursor = len([]rune(derived))
}

func (m *Model) commitForm() {
	title := strings.TrimSpace(m.form.fields[0].value)
	if title == "" {
		return
	}
	path := strings.TrimSpace(m.form.fields[1].value)
	if path == "" {
		path = "."
	}
	tool := m.form.fields[5].options[m.form.fields[5].optIdx]
	flags := strings.TrimSpace(m.form.fields[6].value)
	script := strings.TrimSpace(m.form.fields[7].value)

	if err := db.CreateSession(m.conn, db.Session{
		ID:            uuid.New().String(),
		Title:         title,
		GroupPath:     m.currentGroupPath(),
		ProjectPath:   expandPath(path),
		Tool:          tool,
		ToolFlags:     flags,
		Status:        "stopped",
		CreatedAt:     time.Now().Unix(),
		StartupScript: script,
	}); err != nil {
		m.err = err
	}
}

func (m *Model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.form.formErr = ""
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.mode = ""
		return m, nil

	case tea.KeyEnter:
		m.commitForm()
		if m.form.formErr != "" {
			return m, nil
		}
		m.mode = ""
		if err := m.Reload(); err != nil {
			m.err = err
		}
		return m, nil

	case tea.KeyUp:
		if m.form.focusField > 0 {
			m.form.focusField--
		}
		return m, nil

	case tea.KeyDown:
		if m.form.focusField < len(m.form.fields)-1 {
			m.form.focusField++
		}
		return m, nil

	case tea.KeyTab:
		focus := m.form.focusField
		if focus == 1 {
			m.cyclePathCompletion()
		} else {
			if focus < len(m.form.fields)-1 {
				m.form.focusField++
			}
		}
		return m, nil

	case tea.KeyLeft:
		f := &m.form.fields[m.form.focusField]
		if f.kind == fieldSelect {
			if f.optIdx > 0 {
				f.optIdx--
			} else {
				f.optIdx = len(f.options) - 1
			}
		} else {
			moveCursorLeft(f)
		}
		return m, nil

	case tea.KeyRight:
		f := &m.form.fields[m.form.focusField]
		if f.kind == fieldSelect {
			f.optIdx = (f.optIdx + 1) % len(f.options)
		} else {
			moveCursorRight(f)
		}
		return m, nil

	case tea.KeyBackspace:
		f := &m.form.fields[m.form.focusField]
		if f.kind == fieldText {
			deleteRune(f)
			if m.form.focusField == 1 {
				m.resetPathCompletion()
			}
			if m.form.focusField == 4 {
				m.form.worktreeUserEdited = true
			}
			if m.form.focusField == 2 {
				m.updateWorktreeDefault()
			}
		}
		return m, nil

	case tea.KeySpace:
		f := &m.form.fields[m.form.focusField]
		switch {
		case f.kind == fieldSelect:
			if m.form.focusField < len(m.form.fields)-1 {
				m.form.focusField++
			}
		case m.form.focusField == 1 && m.form.candActive:
			m.resetPathCompletion()
			m.form.focusField = 2
		default:
			insertRune(f, ' ')
		}
		return m, nil

	default:
		if len(msg.Runes) == 1 {
			f := &m.form.fields[m.form.focusField]
			if f.kind == fieldText {
				insertRune(f, msg.Runes[0])
				if m.form.focusField == 1 {
					m.resetPathCompletion()
				}
				if m.form.focusField == 4 {
					m.form.worktreeUserEdited = true
				}
				if m.form.focusField == 2 {
					m.updateWorktreeDefault()
				}
			}
		}
		return m, nil
	}
}

func (m *Model) renderForm() string {
	var sb strings.Builder

	if m.form.focusField == 1 && m.form.candActive && len(m.form.candidates) > 0 {
		var parts []string
		for i, c := range m.form.candidates {
			if i == m.form.candIdx {
				parts = append(parts, formCandHLStyle.Render(c))
			} else {
				parts = append(parts, formCandStyle.Render(c))
			}
		}
		sb.WriteString(formCandStyle.Render("  ") + strings.Join(parts, formCandStyle.Render("  ")) + "\n")
	}

	sb.WriteString(formHeaderStyle.Render("NEW SESSION") + "\n")

	for i, f := range m.form.fields {
		isActive := i == m.form.focusField
		branchEmpty := m.form.fields[2].value == ""
		inertField := (i == 3 || i == 4) && branchEmpty

		var labelStr string
		if isActive && !inertField {
			labelStr = formBarStyle.Render("▌ ") + formLabelActive.Render(f.label)
		} else if isActive && inertField {
			labelStr = formBarStyle.Render("▌ ") + formLabelDim.Render(f.label)
		} else {
			labelStr = "  " + formLabelDim.Render(f.label)
		}
		label := labelStr + "  "

		var valueStr string
		switch f.kind {
		case fieldSelect:
			var parts []string
			for j, opt := range f.options {
				if j == f.optIdx {
					parts = append(parts, selectedStyle.Render("["+opt+"]"))
				} else {
					parts = append(parts, formValueDim.Render(opt))
				}
			}
			valueStr = strings.Join(parts, "  ")
		case fieldText:
			if f.value == "" && !isActive {
				valueStr = formValueDim.Render("─")
			} else if isActive {
				valueStr = renderWithCursor(f.value, f.cursor)
			} else {
				valueStr = formValueDim.Render(f.value)
			}
		}

		sb.WriteString(label + valueStr + "\n")
	}

	if m.form.formErr != "" {
		sb.WriteString(formErrStyle.Render("  "+m.form.formErr) + "\n")
	}
	sb.WriteString(formHintStyle.Render("  Tab · Space · Enter to create · Esc cancel"))
	return sb.String()
}
