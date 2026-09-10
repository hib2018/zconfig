package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/hib2018/zconfig/tui/internal/review"
)

type ReviewModel struct {
	State      review.State
	Monochrome bool
}

func NewReviewModel(state review.State) *ReviewModel {
	return &ReviewModel{State: state}
}

func (m *ReviewModel) Init() tea.Cmd { return nil }

func (m *ReviewModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.State.Width, m.State.Height = msg.Width, msg.Height
		m.State.Move(0)
	case tea.KeyPressMsg:
		key := msg.Keystroke()
		if m.State.EditingFilter {
			switch key {
			case "enter":
				m.State.EditingFilter = false
				m.State.Selected, m.State.Offset = 0, 0
			case "esc":
				m.State.EditingFilter = false
			case "backspace":
				if m.State.Filter != "" {
					_, size := utf8.DecodeLastRuneInString(m.State.Filter)
					m.State.Filter = m.State.Filter[:len(m.State.Filter)-size]
				}
			default:
				if msg.Text != "" && !strings.ContainsAny(msg.Text, "\r\n") {
					m.State.Filter += msg.Text
				}
			}
			return m, nil
		}
		switch key {
		case "j", "down":
			m.State.Move(1)
		case "k", "up":
			m.State.Move(-1)
		case "enter":
			m.State.Mode = review.ModeDetail
		case "d":
			m.State.Mode = review.ModeDiff
		case "?":
			m.State.Mode = review.ModeHelp
		case "esc":
			m.State.Mode = review.ModeList
		case "/":
			m.State.Filter = ""
			m.State.EditingFilter = true
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *ReviewModel) View() tea.View {
	return tea.NewView(m.render())
}

func (m *ReviewModel) render() string {
	width := m.State.Width
	if width < 1 {
		width = 80
	}
	var lines []string
	lines = append(lines, "zconfig review")
	if m.State.Error != "" {
		lines = append(lines, "ERROR: "+m.State.Error)
	}
	if m.State.EditingFilter || m.State.Filter != "" {
		lines = append(lines, "Filter: "+m.State.Filter)
	}
	items := m.State.VisibleItems()
	if len(items) == 0 {
		lines = append(lines, "No change items")
		return fit(lines, width, m.State.Height)
	}
	switch m.State.Mode {
	case review.ModeHelp:
		lines = append(lines, "Help: j/k move  enter details  d diff  / filter  q quit")
	case review.ModeDetail:
		item := items[min(m.State.Selected, len(items)-1)]
		lines = append(lines, item.Path, "Operation: "+string(item.Operation), "Current: "+displayValue(item.ExpectedOld, item.Sensitivity), "Proposed: "+displayValue(item.ProposedValue, item.Sensitivity), item.Explanation)
	case review.ModeDiff:
		item := items[min(m.State.Selected, len(items)-1)]
		lines = append(lines, "Diff "+item.Path, "- "+displayValue(item.ExpectedOld, item.Sensitivity), "+ "+displayValue(item.ProposedValue, item.Sensitivity))
	default:
		page := m.State.Height - len(lines) - 1
		if page < 1 {
			page = 1
		}
		start := min(m.State.Offset, len(items)-1)
		end := min(start+page, len(items))
		for i := start; i < end; i++ {
			item := items[i]
			cursor := "  "
			if i == m.State.Selected {
				cursor = "> "
			}
			status := ""
			for _, check := range item.Checks {
				status += " " + strings.ToUpper(string(check.Status))
			}
			lines = append(lines, fmt.Sprintf("%s%s %s -> %s%s", cursor, item.Path, displayValue(item.ExpectedOld, item.Sensitivity), displayValue(item.ProposedValue, item.Sensitivity), status))
		}
	}
	return fit(lines, width, m.State.Height)
}

func displayValue(raw *json.RawMessage, sensitivity review.Sensitivity) string {
	if (sensitivity == review.SensitivitySchema || sensitivity == review.SensitivitySuspected) && raw != nil {
		return "[REDACTED]"
	}
	if raw == nil {
		return "—"
	}
	return string(*raw)
}

func fit(lines []string, width, height int) string {
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		runes := []rune(line)
		if len(runes) > width {
			if width > 1 {
				line = string(runes[:width-1]) + "…"
			} else {
				line = string(runes[:width])
			}
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}
