package review

import (
	"encoding/json"
	"errors"
	"time"
)

type Operation string

const (
	OperationAdd     Operation = "add"
	OperationReplace Operation = "replace"
	OperationRemove  Operation = "remove"
)

type Decision string

const (
	DecisionPending  Decision = "pending"
	DecisionApproved Decision = "approved"
	DecisionRejected Decision = "rejected"
)

type Sensitivity string

const (
	SensitivityNormal    Sensitivity = "normal"
	SensitivitySchema    Sensitivity = "schema_sensitive"
	SensitivitySuspected Sensitivity = "suspected_sensitive"
)

type CheckStatus string

const (
	CheckPassed     CheckStatus = "passed"
	CheckFailed     CheckStatus = "failed"
	CheckUnverified CheckStatus = "unverified"
)

type CommentStatus string

const (
	CommentOpen           CommentStatus = "open"
	CommentAgentClaimed   CommentStatus = "agent_claimed"
	CommentHumanConfirmed CommentStatus = "human_confirmed"
)

type Lifecycle string

const (
	LifecycleReviewing Lifecycle = "reviewing"
	LifecycleReady     Lifecycle = "ready"
	LifecycleApplied   Lifecycle = "applied"
	LifecycleAbandoned Lifecycle = "abandoned"
	LifecycleApplying  Lifecycle = "applying"
)

type Source struct {
	Path       string     `json:"path"`
	Digest     string     `json:"digest"`
	ByteLength int64      `json:"byte_length"`
	NodeCount  int        `json:"node_count"`
	RootType   string     `json:"root_type"`
	SchemaRef  *SchemaRef `json:"schema_ref,omitempty"`
}
type SchemaRef struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}
type Proposal struct {
	ProposalID   string       `json:"proposal_id"`
	Revision     int          `json:"revision"`
	SourceDigest string       `json:"source_digest"`
	CreatedAt    time.Time    `json:"created_at"`
	Producer     string       `json:"producer,omitempty"`
	Items        []ChangeItem `json:"items"`
}
type ChangeItem struct {
	ChangeID      string           `json:"change_id"`
	Path          string           `json:"path"`
	Operation     Operation        `json:"operation"`
	ExpectedOld   *json.RawMessage `json:"expected_old,omitempty"`
	ProposedValue *json.RawMessage `json:"proposed_value,omitempty"`
	Explanation   string           `json:"explanation"`
	Decision      Decision         `json:"decision"`
	Sensitivity   Sensitivity      `json:"sensitivity"`
	Checks        []CheckResult    `json:"checks"`
}
type CheckResult struct {
	Kind           string      `json:"kind"`
	Status         CheckStatus `json:"status"`
	Code           string      `json:"code"`
	Message        string      `json:"message"`
	Pointer        string      `json:"pointer,omitempty"`
	EvidenceDigest string      `json:"evidence_digest,omitempty"`
}
type Comment struct {
	CommentID string        `json:"comment_id"`
	ChangeID  string        `json:"change_id"`
	Body      string        `json:"body"`
	Status    CommentStatus `json:"status"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}
type Revision struct {
	ProposalID         string
	BaseRevision       int
	SourceDigest       string
	AllowedChangeIDs   []string
	Proposal           Proposal
	ResolvedCommentIDs []string
}
type AuditEvent struct {
	EventID      string
	SessionID    string
	Sequence     int
	OccurredAt   time.Time
	ActorType    string
	Action       string
	ChangeIDs    []string
	SourceDigest string
}
type Session struct {
	SessionSchema  string    `json:"session_schema"`
	ReviewID       string    `json:"review_id"`
	Source         Source    `json:"source"`
	ActiveProposal Proposal  `json:"active_proposal"`
	Comments       []Comment `json:"comments"`
	Lifecycle      Lifecycle `json:"lifecycle"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (i ChangeItem) Validate() error {
	if i.ChangeID == "" {
		return errors.New("missing change id")
	}
	switch i.Operation {
	case OperationAdd:
		if i.ExpectedOld != nil || i.ProposedValue == nil {
			return errors.New("invalid add")
		}
	case OperationReplace:
		if i.ExpectedOld == nil || i.ProposedValue == nil {
			return errors.New("invalid replace")
		}
	case OperationRemove:
		if i.ExpectedOld == nil || i.ProposedValue != nil {
			return errors.New("invalid remove")
		}
	default:
		return errors.New("invalid operation")
	}
	return nil
}
func (s Session) Validate() error {
	switch s.Lifecycle {
	case LifecycleReviewing, LifecycleReady, LifecycleApplied, LifecycleAbandoned:
		return nil
	}
	return errors.New("lifecycle cannot be persisted")
}
