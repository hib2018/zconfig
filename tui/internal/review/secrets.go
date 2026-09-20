package review

import (
	"errors"
	"time"
)

type SecretCapability string

const (
	SecretReveal    SecretCapability = "reveal"
	SecretShareOnce SecretCapability = "share_once"
)

type SecretAuthorization struct {
	ChangeID    string
	Fingerprint string
	Capability  SecretCapability
	ExpiresAt   time.Time
	Consumed    bool
}

type SecretStore struct {
	authorizations map[string]*SecretAuthorization
}

func NewSecretStore() *SecretStore {
	return &SecretStore{authorizations: map[string]*SecretAuthorization{}}
}

func (s *SecretStore) Grant(changeID, fingerprint string, capability SecretCapability, expiresAt time.Time) error {
	if changeID == "" || fingerprint == "" || (capability != SecretReveal && capability != SecretShareOnce) {
		return errors.New("invalid secret authorization")
	}
	s.authorizations[string(capability)+"\x00"+changeID] = &SecretAuthorization{ChangeID: changeID, Fingerprint: fingerprint, Capability: capability, ExpiresAt: expiresAt}
	return nil
}

func (s *SecretStore) Authorize(changeID, fingerprint string, capability SecretCapability, now time.Time) error {
	a := s.authorizations[string(capability)+"\x00"+changeID]
	if a == nil || a.Consumed || now.After(a.ExpiresAt) || a.Fingerprint != fingerprint {
		return errors.New("secret authorization unavailable")
	}
	if capability == SecretShareOnce {
		a.Consumed = true
	}
	return nil
}

func (s *SecretStore) Conceal(changeID string) {
	delete(s.authorizations, string(SecretReveal)+"\x00"+changeID)
	delete(s.authorizations, string(SecretShareOnce)+"\x00"+changeID)
}

func (s *SecretStore) Reset() { clear(s.authorizations) }
