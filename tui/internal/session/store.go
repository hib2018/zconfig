package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hib2018/zconfig/tui/internal/review"
)

const SessionSchema = "zconfig.review-session/1"

func Path(projectDirectory, reviewID string) string {
	return filepath.Join(projectDirectory, ".zconfig", "reviews", reviewID+".json")
}

func Save(projectDirectory string, value review.Session) error {
	if err := validateSession(value); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	destination := Path(projectDirectory, value.ReviewID)
	dir := filepath.Dir(destination)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".session-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	ok := false
	defer func() {
		_ = temporary.Close()
		if !ok {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, destination); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return err
	}
	ok = true
	return nil
}

func Load(projectDirectory, reviewID string) (review.Session, error) {
	if !safeID(reviewID) {
		return review.Session{}, errors.New("invalid review id")
	}
	data, err := os.ReadFile(Path(projectDirectory, reviewID))
	if err != nil {
		return review.Session{}, err
	}
	var value review.Session
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return review.Session{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return review.Session{}, errors.New("session has extra JSON data")
	}
	if value.ReviewID != reviewID {
		return review.Session{}, errors.New("session id does not match filename")
	}
	if err := validateSession(value); err != nil {
		return review.Session{}, err
	}
	return value, nil
}

func validateSession(value review.Session) error {
	if value.SessionSchema != SessionSchema {
		return errors.New("unsupported session schema")
	}
	if !safeID(value.ReviewID) {
		return errors.New("invalid review id")
	}
	if value.Source.Path == "" || !validDigest(value.Source.Digest) {
		return errors.New("invalid source reference")
	}
	if value.ActiveProposal.ProposalID == "" || value.ActiveProposal.SourceDigest != value.Source.Digest {
		return errors.New("invalid active proposal")
	}
	if err := value.Validate(); err != nil {
		return err
	}
	for _, item := range value.ActiveProposal.Items {
		if item.Sensitivity != "" && item.Sensitivity != review.SensitivityNormal && (item.ExpectedOld != nil || item.ProposedValue != nil) {
			return fmt.Errorf("sensitive values may not be persisted for change %q", item.ChangeID)
		}
	}
	return nil
}

func safeID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func validDigest(value string) bool {
	if len(value) != 71 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, r := range value[7:] {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
