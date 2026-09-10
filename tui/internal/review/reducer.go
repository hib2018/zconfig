package review

import "strings"

type Mode string

const (
	ModeList   Mode = "list"
	ModeDetail Mode = "detail"
	ModeDiff   Mode = "diff"
	ModeHelp   Mode = "help"
)

type State struct {
	Items         []ChangeItem
	Comments      []Comment
	Selected      int
	Offset        int
	Width         int
	Height        int
	Mode          Mode
	Filter        string
	EditingFilter bool
	Error         string
}

func NewState(items []ChangeItem) State {
	copyItems := append([]ChangeItem(nil), items...)
	for i := range copyItems {
		if copyItems[i].Decision == "" {
			copyItems[i].Decision = DecisionPending
		}
	}
	return State{Items: copyItems, Width: 80, Height: 24, Mode: ModeList}
}

func (s *State) VisibleItems() []ChangeItem {
	if s.Filter == "" {
		return s.Items
	}
	needle := strings.ToLower(s.Filter)
	visible := make([]ChangeItem, 0, len(s.Items))
	for _, item := range s.Items {
		if strings.Contains(strings.ToLower(item.ChangeID+" "+item.Path+" "+item.Explanation), needle) {
			visible = append(visible, item)
		}
	}
	return visible
}

func (s *State) Move(delta int) {
	count := len(s.VisibleItems())
	if count == 0 {
		s.Selected, s.Offset = 0, 0
		return
	}
	s.Selected += delta
	if s.Selected < 0 {
		s.Selected = 0
	}
	if s.Selected >= count {
		s.Selected = count - 1
	}
	page := s.Height - 4
	if page < 1 {
		page = 1
	}
	if s.Selected < s.Offset {
		s.Offset = s.Selected
	}
	if s.Selected >= s.Offset+page {
		s.Offset = s.Selected - page + 1
	}
}
