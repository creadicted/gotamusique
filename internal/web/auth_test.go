package web

import (
	"errors"
	"strings"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestAuth() (*Auth, *clock) {
	c := &clock{t: time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)}
	return NewAuth(AuthOptions{Now: c.now}), c
}

var alice = Identity{Subject: "hash-alice", Name: "alice", Roles: []Role{RoleUser}}

func TestToken_WorksExactlyOnce(t *testing.T) {
	a, _ := newTestAuth()
	tok, err := a.IssueToken(alice)
	if err != nil {
		t.Fatal(err)
	}
	sid, id, err := a.Redeem(tok)
	if err != nil || id.Name != "alice" || sid == "" {
		t.Fatalf("Redeem = %q, %+v, %v", sid, id, err)
	}
	if _, _, err := a.Redeem(tok); !errors.Is(err, ErrBadToken) {
		t.Errorf("second Redeem = %v, want ErrBadToken", err)
	}
}

func TestToken_ExpiresAndUnknownIsRejected(t *testing.T) {
	a, c := newTestAuth()
	tok, _ := a.IssueToken(alice)
	c.advance(DefaultTokenTTL)
	if _, _, err := a.Redeem(tok); !errors.Is(err, ErrBadToken) {
		t.Errorf("expired token = %v, want ErrBadToken", err)
	}
	for _, bad := range []string{"", "nope", strings.Repeat("a", 43)} {
		if _, _, err := a.Redeem(bad); !errors.Is(err, ErrBadToken) {
			t.Errorf("Redeem(%q) = %v", bad, err)
		}
	}
}

func TestToken_NewRequestInvalidatesTheOldOne(t *testing.T) {
	a, _ := newTestAuth()
	first, _ := a.IssueToken(alice)
	second, _ := a.IssueToken(alice)
	if first == second {
		t.Fatal("tokens must be random")
	}
	if _, _, err := a.Redeem(first); !errors.Is(err, ErrBadToken) {
		t.Error("the older token must stop working")
	}
	if _, _, err := a.Redeem(second); err != nil {
		t.Errorf("the newest token = %v", err)
	}
}

func TestToken_PendingTokensAreBounded(t *testing.T) {
	a, _ := newTestAuth()
	var lastErr error
	for i := 0; i <= maxTokens+1; i++ {
		_, lastErr = a.IssueToken(Identity{Subject: "s" + string(rune('A'+i%26)) + strings.Repeat("x", i), Name: "u"})
	}
	if lastErr == nil {
		t.Error("expected the pending-token cap to refuse new tokens")
	}
}

func TestSession_LookupSlidesIdleExpiry(t *testing.T) {
	a, c := newTestAuth()
	tok, _ := a.IssueToken(alice)
	sid, _, _ := a.Redeem(tok)

	for i := 0; i < 3; i++ { // each use pushes the idle deadline out
		c.advance(DefaultSessionTTL - time.Minute)
		if _, ok := a.Lookup(sid); !ok {
			t.Fatalf("session expired too early at step %d", i)
		}
	}
	c.advance(DefaultSessionTTL)
	if _, ok := a.Lookup(sid); ok {
		t.Error("an idle session must expire")
	}
}

func TestSession_AbsoluteLifetimeCapsASlidingSession(t *testing.T) {
	a, c := newTestAuth()
	tok, _ := a.IssueToken(alice)
	sid, _, _ := a.Redeem(tok)
	for c.t.Sub(time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)) < DefaultSessionMax+time.Hour {
		c.advance(time.Hour)
		if _, ok := a.Lookup(sid); !ok {
			return // expired at the cap, as it should
		}
	}
	t.Error("a continuously used session must still end at the absolute cap")
}

func TestSession_LogoutAndUnknownID(t *testing.T) {
	a, _ := newTestAuth()
	tok, _ := a.IssueToken(alice)
	sid, _, _ := a.Redeem(tok)
	a.Logout(sid)
	if _, ok := a.Lookup(sid); ok {
		t.Error("session survived logout")
	}
	if _, ok := a.Lookup("garbage"); ok {
		t.Error("unknown session accepted")
	}
}

func TestRevokeSubject_EndsSessionsAndPendingTokens(t *testing.T) {
	a, _ := newTestAuth()
	tok, _ := a.IssueToken(alice)
	sid, _, _ := a.Redeem(tok)
	pending, _ := a.IssueToken(alice)
	bobTok, _ := a.IssueToken(Identity{Subject: "hash-bob", Name: "bob", Roles: []Role{RoleUser}})
	bobSid, _, _ := a.Redeem(bobTok)

	a.RevokeSubject("hash-alice")
	if _, ok := a.Lookup(sid); ok {
		t.Error("revoked session still valid")
	}
	if _, _, err := a.Redeem(pending); !errors.Is(err, ErrBadToken) {
		t.Error("revoked pending token still valid")
	}
	if _, ok := a.Lookup(bobSid); !ok {
		t.Error("revoking one subject must not touch others")
	}
}

func TestSecrets_AreNotStoredInTheClear(t *testing.T) {
	a, _ := newTestAuth()
	tok, _ := a.IssueToken(alice)
	sid, _, _ := a.Redeem(tok)
	a.mu.Lock()
	defer a.mu.Unlock()
	for h := range a.sessions {
		if string(h[:]) == sid {
			t.Error("session ID stored in the clear")
		}
	}
	if len(tok) < 40 || len(sid) < 40 {
		t.Errorf("secrets too short: token %d, session %d chars", len(tok), len(sid))
	}
}

func TestIdentity_Has(t *testing.T) {
	id := Identity{Roles: []Role{RoleUser}}
	if !id.Has(RoleUser) || id.Has(RoleAdmin) {
		t.Error("Has wrong")
	}
}
