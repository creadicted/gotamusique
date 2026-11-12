package web

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Role is what a session may do. Handlers check roles, never the auth method, so
// another way to log in (OIDC, say) only has to produce a session with roles.
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// Identity is who a session belongs to.
type Identity struct {
	// Subject is stable and unique: the certificate hash for a registered
	// Mumble user, otherwise "name:<name>".
	Subject string
	// Name is the display name.
	Name  string
	Roles []Role
}

// Has reports whether the identity holds role.
func (i Identity) Has(role Role) bool {
	for _, r := range i.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// Auth timing and limits.
const (
	// DefaultTokenTTL is how long a !web link stays valid.
	DefaultTokenTTL = 5 * time.Minute
	// DefaultSessionTTL is the idle time after which a session expires; every use
	// slides it forward.
	DefaultSessionTTL = 12 * time.Hour
	// DefaultSessionMax is the absolute lifetime of a session, however active.
	DefaultSessionMax = 7 * 24 * time.Hour

	maxTokens   = 1024 // outstanding login tokens
	maxSessions = 4096
)

// ErrDisabled is returned by the bot's login-link request when the web interface
// is not enabled.
var ErrDisabled = errors.New("the web interface is disabled")

// LoginURL builds the link that logs a user in with token, under the public
// access address.
func LoginURL(accessAddress, token string) string {
	return strings.TrimRight(accessAddress, "/") + "/login?token=" + url.QueryEscape(token)
}

// ErrBadToken is returned by Redeem for an unknown, expired or already used token.
var ErrBadToken = errors.New("invalid or expired token")

// Auth issues one-time login tokens and keeps the sessions they turn into. It is
// safe for concurrent use. Everything lives in memory: a restart logs everyone out.
//
// Tokens and session IDs are 256 random bits, so guessing one is not a practical
// attack and no attempt limiting is needed. Only their SHA-256 is stored, so a
// memory dump does not hand out usable credentials.
type Auth struct {
	tokenTTL, sessionTTL, sessionMax time.Duration
	now                              func() time.Time

	mu       sync.Mutex
	tokens   map[[32]byte]pendingToken
	bySubj   map[string][32]byte // the one outstanding token per subject
	sessions map[[32]byte]*session
}

type pendingToken struct {
	id      Identity
	expires time.Time
}

type session struct {
	id      Identity
	created time.Time
	seen    time.Time
}

// AuthOptions overrides the defaults; zero fields keep them.
type AuthOptions struct {
	TokenTTL, SessionTTL, SessionMax time.Duration
	Now                              func() time.Time // tests
}

// NewAuth returns an empty Auth.
func NewAuth(o AuthOptions) *Auth {
	a := &Auth{
		tokenTTL: o.TokenTTL, sessionTTL: o.SessionTTL, sessionMax: o.SessionMax, now: o.Now,
		tokens:   map[[32]byte]pendingToken{},
		bySubj:   map[string][32]byte{},
		sessions: map[[32]byte]*session{},
	}
	if a.tokenTTL <= 0 {
		a.tokenTTL = DefaultTokenTTL
	}
	if a.sessionTTL <= 0 {
		a.sessionTTL = DefaultSessionTTL
	}
	if a.sessionMax <= 0 {
		a.sessionMax = DefaultSessionMax
	}
	if a.now == nil {
		a.now = time.Now
	}
	return a
}

// TokenTTL is how long an issued token is valid.
func (a *Auth) TokenTTL() time.Duration { return a.tokenTTL }

func newSecret() (secret string, hash [32]byte, err error) {
	var raw [32]byte
	if _, err = rand.Read(raw[:]); err != nil {
		return "", hash, err
	}
	secret = base64.RawURLEncoding.EncodeToString(raw[:])
	return secret, sha256.Sum256([]byte(secret)), nil
}

func hashOf(secret string) [32]byte { return sha256.Sum256([]byte(secret)) }

// IssueToken creates a single-use login token for id. A subject has at most one
// outstanding token: asking again invalidates the previous one.
func (a *Auth) IssueToken(id Identity) (string, error) {
	secret, h, err := newSecret()
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	a.sweepLocked(now)
	if old, ok := a.bySubj[id.Subject]; ok {
		delete(a.tokens, old)
	}
	if len(a.tokens) >= maxTokens {
		return "", errors.New("too many pending logins, try again in a few minutes")
	}
	a.tokens[h] = pendingToken{id: id, expires: now.Add(a.tokenTTL)}
	a.bySubj[id.Subject] = h
	return secret, nil
}

// Redeem consumes token and starts a session, returning the session ID for the
// cookie. A token works exactly once.
func (a *Auth) Redeem(token string) (sessionID string, id Identity, err error) {
	th := hashOf(token)
	secret, sh, err := newSecret()
	if err != nil {
		return "", Identity{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	p, ok := a.tokens[th]
	if !ok {
		return "", Identity{}, ErrBadToken
	}
	delete(a.tokens, th)
	if a.bySubj[p.id.Subject] == th {
		delete(a.bySubj, p.id.Subject)
	}
	if !now.Before(p.expires) {
		return "", Identity{}, ErrBadToken
	}
	a.sweepLocked(now)
	if len(a.sessions) >= maxSessions {
		a.evictOldestLocked()
	}
	a.sessions[sh] = &session{id: p.id, created: now, seen: now}
	return secret, p.id, nil
}

// Lookup returns the identity behind a session ID and slides the session's idle
// expiry. ok is false for an unknown or expired session.
func (a *Auth) Lookup(sessionID string) (Identity, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	h := hashOf(sessionID)
	s, ok := a.sessions[h]
	if !ok {
		return Identity{}, false
	}
	now := a.now()
	if a.expired(s, now) {
		delete(a.sessions, h)
		return Identity{}, false
	}
	s.seen = now
	return s.id, true
}

// Logout ends one session.
func (a *Auth) Logout(sessionID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, hashOf(sessionID))
}

// RevokeSubject ends every session and pending token of subject, for example
// when a user is banned.
func (a *Auth) RevokeSubject(subject string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for h, s := range a.sessions {
		if s.id.Subject == subject {
			delete(a.sessions, h)
		}
	}
	if h, ok := a.bySubj[subject]; ok {
		delete(a.tokens, h)
		delete(a.bySubj, subject)
	}
}

func (a *Auth) expired(s *session, now time.Time) bool {
	return now.Sub(s.seen) >= a.sessionTTL || now.Sub(s.created) >= a.sessionMax
}

// sweepLocked drops expired tokens and sessions. It runs on issue and redeem, so
// memory stays bounded without a background goroutine.
func (a *Auth) sweepLocked(now time.Time) {
	for h, p := range a.tokens {
		if !now.Before(p.expires) {
			delete(a.tokens, h)
			if a.bySubj[p.id.Subject] == h {
				delete(a.bySubj, p.id.Subject)
			}
		}
	}
	for h, s := range a.sessions {
		if a.expired(s, now) {
			delete(a.sessions, h)
		}
	}
}

func (a *Auth) evictOldestLocked() {
	var oldest [32]byte
	var at time.Time
	first := true
	for h, s := range a.sessions {
		if first || s.seen.Before(at) {
			oldest, at, first = h, s.seen, false
		}
	}
	if !first {
		delete(a.sessions, oldest)
	}
}
