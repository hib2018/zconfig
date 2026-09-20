package review

import (
	"errors"
	"strings"
)

type Mode string

const (
	ModeList     Mode = "list"
	ModeDetail   Mode = "detail"
	ModeDiff     Mode = "diff"
	ModeHelp     Mode = "help"
	ModeApproval Mode = "approval"
)

type State struct {
	Items     []ChangeItem
	Comments  []Comment
	Lifecycle Lifecycle
	// FinalConfirmationDigest is deliberately ephemeral. Session persistence must
	// never restore it, and every authoritative review change clears it.
	FinalConfirmationDigest string
	Selected                int
	Offset                  int
	Width                   int
	Height                  int
	Mode                    Mode
	Filter                  string
	EditingFilter           bool
	Error                   string
	OnStableTransition      func(State) error
}

func RestoreSession(session Session, currentSourceDigest string) (State, error) {
	if session.Source.Digest != currentSourceDigest {
		return State{}, errors.New("source digest changed since session save")
	}
	state := NewState(session.ActiveProposal.Items)
	state.Comments = append([]Comment(nil), session.Comments...)
	state.Lifecycle = session.Lifecycle
	state.FinalConfirmationDigest = ""
	return state, nil
}

func (s *State) ReconcileInterruptedApply(sourceDigestBefore, sourceDigestAfter, currentSourceDigest string) error {
	s.FinalConfirmationDigest = ""
	switch currentSourceDigest {
	case sourceDigestBefore:
		s.Lifecycle = LifecycleReady
	case sourceDigestAfter:
		s.Lifecycle = LifecycleApplied
	default:
		s.Lifecycle = LifecycleReviewing
		return errors.New("source digest does not match pre-apply or applied content")
	}
	return s.stableTransition()
}

func (s *State) stableTransition() error {
	if s.OnStableTransition == nil {
		return nil
	}
	if err := s.OnStableTransition(*s); err != nil {
		s.Error = "autosave failed: " + err.Error()
		return err
	}
	return nil
}

func (s *State) SelectedItem() *ChangeItem {
	visible := s.VisibleItems()
	if len(visible) == 0 {
		return nil
	}
	id := visible[min(s.Selected, len(visible)-1)].ChangeID
	for i := range s.Items {
		if s.Items[i].ChangeID == id {
			return &s.Items[i]
		}
	}
	return nil
}

func NewState(items []ChangeItem) State {
	copyItems := append([]ChangeItem(nil), items...)
	for i := range copyItems {
		if copyItems[i].Decision == "" {
			copyItems[i].Decision = DecisionPending
		}
	}
	return State{Items: copyItems, Lifecycle: LifecycleReviewing, Width: 80, Height: 24, Mode: ModeList}
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
