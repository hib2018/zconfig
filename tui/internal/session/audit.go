package session

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const AuditSchema = "zconfig.audit-event/1"
const maxAuditEvents = 5000

type AuditEvent struct {
	AuditSchema         string    `json:"audit_schema"`
	EventID             string    `json:"event_id"`
	SessionID           string    `json:"session_id"`
	Sequence            int       `json:"sequence"`
	OccurredAt          time.Time `json:"occurred_at"`
	ActorType           string    `json:"actor_type"`
	Action              string    `json:"action"`
	ChangeIDs           []string  `json:"change_ids"`
	CommentIDs          []string  `json:"comment_ids,omitempty"`
	SourceDigest        string    `json:"source_digest"`
	ProposalDigest      string    `json:"proposal_digest,omitempty"`
	OutcomeCode         string    `json:"outcome_code,omitempty"`
	PreviousEventDigest *string   `json:"previous_event_digest"`
	EventDigest         string    `json:"event_digest"`
}

type digestFields struct {
	AuditSchema         string    `json:"audit_schema"`
	EventID             string    `json:"event_id"`
	SessionID           string    `json:"session_id"`
	Sequence            int       `json:"sequence"`
	OccurredAt          time.Time `json:"occurred_at"`
	ActorType           string    `json:"actor_type"`
	Action              string    `json:"action"`
	ChangeIDs           []string  `json:"change_ids"`
	CommentIDs          []string  `json:"comment_ids,omitempty"`
	SourceDigest        string    `json:"source_digest"`
	ProposalDigest      string    `json:"proposal_digest,omitempty"`
	OutcomeCode         string    `json:"outcome_code,omitempty"`
	PreviousEventDigest *string   `json:"previous_event_digest"`
}

type Audit struct {
	path       string
	sessionID  string
	sequence   int
	previous   string
	appendLine func([]byte) error
}

func OpenAudit(projectDirectory, sessionID string) (*Audit, error) {
	if !safeID(sessionID) {
		return nil, errors.New("invalid audit session id")
	}
	dir := filepath.Join(projectDirectory, ".zconfig", "audit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, sessionID+".jsonl")
	events, err := verifyAuditIfExists(path, sessionID)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	a := &Audit{path: path, sessionID: sessionID}
	if len(events) != 0 {
		a.sequence = events[len(events)-1].Sequence
		a.previous = events[len(events)-1].EventDigest
	}
	a.appendLine = func(line []byte) error {
		writer, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer writer.Close()
		if _, err := writer.Write(line); err != nil {
			return err
		}
		return writer.Sync()
	}
	return a, nil
}

func (a *Audit) Path() string { return a.path }

func (a *Audit) Append(event AuditEvent) error {
	if a.appendLine == nil {
		return errors.New("audit writer is unavailable")
	}
	event.AuditSchema, event.SessionID, event.Sequence = AuditSchema, a.sessionID, a.sequence+1
	if a.previous != "" {
		previous := a.previous
		event.PreviousEventDigest = &previous
	} else {
		event.PreviousEventDigest = nil
	}
	if err := validateAuditEvent(event); err != nil {
		return err
	}
	digest, err := eventDigest(event)
	if err != nil {
		return err
	}
	event.EventDigest = digest
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if err := a.appendLine(line); err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}
	a.sequence, a.previous = event.Sequence, event.EventDigest
	return nil
}

func (a *Audit) Authorize(event AuditEvent, action func() error) error {
	if err := a.Append(event); err != nil {
		return err
	}
	return action()
}

func VerifyAudit(path, sessionID string) ([]AuditEvent, error) {
	return verifyAuditIfExists(path, sessionID)
}

func verifyAuditIfExists(path, sessionID string) ([]AuditEvent, error) {
	contents, readErr := os.ReadFile(path)
	if readErr == nil && len(contents) != 0 && contents[len(contents)-1] != '\n' {
		return nil, errors.New("audit log has an incomplete final line")
	}
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return nil, readErr
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var events []AuditEvent
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var previous string
	for scanner.Scan() {
		if len(events) >= maxAuditEvents {
			return nil, errors.New("audit event limit exceeded")
		}
		var event AuditEvent
		decoder := json.NewDecoder(strings.NewReader(scanner.Text()))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&event); err != nil {
			return nil, err
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return nil, errors.New("audit line has extra JSON data")
		}
		if event.SessionID != sessionID || event.Sequence != len(events)+1 {
			return nil, errors.New("invalid audit identity or sequence")
		}
		if event.Sequence == 1 {
			if event.PreviousEventDigest != nil {
				return nil, errors.New("first audit event has previous digest")
			}
		} else if event.PreviousEventDigest == nil || *event.PreviousEventDigest != previous {
			return nil, errors.New("broken audit chain")
		}
		if err := validateAuditEvent(event); err != nil {
			return nil, err
		}
		want, err := eventDigest(event)
		if err != nil || event.EventDigest != want {
			return nil, errors.New("invalid audit event digest")
		}
		previous = event.EventDigest
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func eventDigest(event AuditEvent) (string, error) {
	fields := digestFields{event.AuditSchema, event.EventID, event.SessionID, event.Sequence, event.OccurredAt, event.ActorType, event.Action, event.ChangeIDs, event.CommentIDs, event.SourceDigest, event.ProposalDigest, event.OutcomeCode, event.PreviousEventDigest}
	data, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

var actions = []string{"review.started", "review.resumed", "review.ended", "proposal.loaded", "comment.created", "comment.updated", "comment.withdrawn", "revision.requested", "revision.accepted", "revision.rejected", "validation.completed", "item.approved", "item.rejected", "item.reset", "secret.revealed", "secret.shared_once", "final.previewed", "final.confirmed", "apply.started", "apply.succeeded", "apply.failed"}

func validateAuditEvent(event AuditEvent) error {
	if event.AuditSchema != AuditSchema || !safeID(event.EventID) || !safeID(event.SessionID) || event.Sequence < 1 || event.Sequence > maxAuditEvents {
		return errors.New("invalid audit envelope")
	}
	if event.OccurredAt.IsZero() || (event.ActorType != "human" && event.ActorType != "agent" && event.ActorType != "system") || !slices.Contains(actions, event.Action) || !validDigest(event.SourceDigest) {
		return errors.New("invalid audit fields")
	}
	if event.ProposalDigest != "" && !validDigest(event.ProposalDigest) {
		return errors.New("invalid proposal digest")
	}
	if event.OutcomeCode != "" && !safeCode(event.OutcomeCode) {
		return errors.New("invalid outcome code")
	}
	seen := map[string]bool{}
	for _, id := range append(append([]string{}, event.ChangeIDs...), event.CommentIDs...) {
		if !safeID(id) || seen[id] {
			return errors.New("invalid or duplicate audit id")
		}
		seen[id] = true
	}
	if event.PreviousEventDigest != nil && !validDigest(*event.PreviousEventDigest) {
		return errors.New("invalid previous digest")
	}
	return nil
}

func safeCode(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	segmentStart := true
	for _, r := range value {
		if segmentStart && !(r >= 'a' && r <= 'z') {
			return false
		}
		if r == '.' {
			segmentStart = true
			continue
		}
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_' {
			return false
		}
		segmentStart = false
	}
	return !segmentStart
}
