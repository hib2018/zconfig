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
	view := tea.NewView(m.render())
	view.AltScreen = true
	return view
}

func (m *ReviewModel) render() string {
	width := max(12, m.State.Width)
	height := max(6, m.State.Height)
	items := m.State.VisibleItems()

	header := fmt.Sprintf("ZCONFIG REVIEW  changes %d/%d  mode %s  filter %s", len(items), len(m.State.Items), fallback(string(m.State.Mode), "list"), fallback(m.State.Filter, "-"))
	if m.State.EditingFilter {
		header += "  editing-filter"
	}
	if m.State.Error != "" {
		header += "  ERROR: " + m.State.Error
	}

	footer := "↑/↓ j/k move  enter detail  d diff  / filter  esc list  ? help  q quit"
	bodyHeight := height - 6 // header pane 3 + footer pane 3
	if bodyHeight < 6 {
		bodyHeight = 6
	}

	var body string
	if width < 72 {
		listH := max(4, bodyHeight/2)
		detailH := max(4, bodyHeight-listH)
		body = strings.Join(renderPane("Change List", m.renderList(items, listH-2), width, listH, true), "\n") + "\n" +
			strings.Join(renderPane("Selected Change", m.renderDetail(items), width, detailH, false), "\n")
	} else {
		left := max(30, width*2/5)
		right := width - left - 1
		body = joinPanes(
			renderPane("Change List", m.renderList(items, bodyHeight-2), left, bodyHeight, true),
			renderPane("Selected Change", m.renderDetail(items), right, bodyHeight, false),
		)
	}

	parts := []string{
		strings.Join(renderPane("Review", header, width, 3, false), "\n"),
		body,
		strings.Join(renderPane("Keys", footer, width, 3, false), "\n"),
	}
	return fit(strings.Split(strings.Join(parts, "\n"), "\n"), width, height)
}

func (m *ReviewModel) renderList(items []review.ChangeItem, page int) string {
	if len(items) == 0 {
		return "No change items"
	}
	if page < 1 {
		page = 1
	}
	start := min(m.State.Offset, len(items)-1)
	itemRows := 2
	visible := max(1, page/itemRows)
	end := min(start+visible, len(items))
	var lines []string
	for i := start; i < end; i++ {
		item := items[i]
		marker := "  "
		if i == m.State.Selected {
			marker = "› "
		}
		title := fmt.Sprintf("%s%s  %s", marker, strings.ToUpper(string(item.Operation)), item.Path)
		if i == m.State.Selected && !m.Monochrome {
			title = invert(title)
		}
		lines = append(lines, title, fmt.Sprintf("    %s → %s", displayValue(item.ExpectedOld, item.Sensitivity), displayValue(item.ProposedValue, item.Sensitivity)))
	}
	return strings.Join(lines, "\n")
}

func (m *ReviewModel) renderDetail(items []review.ChangeItem) string {
	if len(items) == 0 {
		return "No change items"
	}
	item := items[min(m.State.Selected, len(items)-1)]
	switch m.State.Mode {
	case review.ModeHelp:
		return "Help\n\nNavigate with j/k or arrows. Enter shows detail, d shows a focused diff, / filters by id/path/explanation, esc returns to list, q quits. Sensitive values are redacted before rendering."
	case review.ModeDiff:
		return strings.Join([]string{
			"Diff " + item.Path,
			"- " + displayValue(item.ExpectedOld, item.Sensitivity),
			"+ " + displayValue(item.ProposedValue, item.Sensitivity),
		}, "\n")
	default:
		var lines []string
		lines = append(lines,
			"ID        : "+item.ChangeID,
			"Path      : "+item.Path,
			"Operation : "+string(item.Operation),
			"Decision  : "+string(item.Decision),
			"Current   : "+displayValue(item.ExpectedOld, item.Sensitivity),
			"Proposed  : "+displayValue(item.ProposedValue, item.Sensitivity),
			"Sensitive : "+fallback(string(item.Sensitivity), string(review.SensitivityNormal)),
		)
		if len(item.Checks) > 0 {
			lines = append(lines, "", "Checks")
			for _, check := range item.Checks {
				lines = append(lines, fmt.Sprintf("- %s %s %s", strings.ToUpper(string(check.Status)), check.Code, check.Message))
			}
		}
		if item.Explanation != "" {
			lines = append(lines, "", "Explanation", item.Explanation)
		}
		return strings.Join(lines, "\n")
	}
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

func checkLabel(item review.ChangeItem) string {
	label := string(item.Decision)
	for _, check := range item.Checks {
		if check.Status == review.CheckFailed {
			return "FAILED"
		}
		if check.Status == review.CheckUnverified {
			label = "UNVERIFIED"
		}
		if check.Status == review.CheckPassed && label == string(review.DecisionPending) {
			label = "PASSED"
		}
	}
	return strings.ToUpper(fallback(label, string(review.DecisionPending)))
}

func renderPane(title, content string, width, height int, focused bool) []string {
	width = max(12, width)
	height = max(3, height)
	inner := width - 2
	label := " " + title + " "
	if focused {
		label = "[ " + title + " ]"
	}
	label = fitLine(label, inner)
	lines := []string{"┌" + label + strings.Repeat("─", max(0, inner-len([]rune(label)))) + "┐"}
	contentLines := wrapContent(content, inner)
	for row := 0; row < height-2; row++ {
		value := ""
		if row < len(contentLines) {
			value = fitLine(contentLines[row], inner)
		}
		lines = append(lines, "│"+value+strings.Repeat(" ", max(0, inner-len([]rune(stripANSI(value)))))+"│")
	}
	return append(lines, "└"+strings.Repeat("─", inner)+"┘")
}

func joinPanes(left, right []string) string {
	rows := min(len(left), len(right))
	joined := make([]string, rows)
	for row := 0; row < rows; row++ {
		joined[row] = left[row] + " " + right[row]
	}
	return strings.Join(joined, "\n")
}

func wrapContent(content string, width int) []string {
	if width <= 0 || content == "" {
		return nil
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSuffix(content, "\n"), "\n") {
		if line == "" {
			out = append(out, "")
			continue
		}
		for len([]rune(stripANSI(line))) > width {
			runes := []rune(line)
			cut := min(width, len(runes))
			out = append(out, string(runes[:cut]))
			line = string(runes[cut:])
		}
		out = append(out, line)
	}
	return out
}

func fit(lines []string, width, height int) string {
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	for i, line := range lines {
		lines[i] = fitLine(line, width)
	}
	return strings.Join(lines, "\n")
}

func fitLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	if len([]rune(stripANSI(line))) <= width {
		return line
	}
	runes := []rune(line)
	if width == 1 {
		return string(runes[:1])
	}
	return string(runes[:min(width-1, len(runes))]) + "…"
}

func fallback(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func invert(value string) string { return "\x1b[7m" + value + "\x1b[0m" }

func stripANSI(value string) string {
	for {
		start := strings.IndexByte(value, '\x1b')
		if start < 0 || start+1 >= len(value) {
			return value
		}
		end := strings.IndexByte(value[start:], 'm')
		if end < 0 {
			return value[:start]
		}
		value = value[:start] + value[start+end+1:]
	}
}
