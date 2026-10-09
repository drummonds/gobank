package staff

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"

	"git.bytestone.uk/hum3/gobank/core"
)

// The staff session (story 1.7.1): who is signed in, carried by a cookie
// the BFF's session store issued, with the role the user holds. Every page
// but the login, the favicon and about.json (which gobank-deploy reads)
// needs one.

const cookieName = "staff_session"

// staffSession is the signed-in user a request carries.
type staffSession struct {
	user  core.User
	role  Role
	token string
}

// key is what the PII authorisation hangs off: the token's hash, so the
// token itself is held nowhere but the cookie.
func (s staffSession) key() string {
	sum := sha256.Sum256([]byte(s.token))
	return hex.EncodeToString(sum[:])
}

type sessionKey struct{}

// signedIn is the session the request carries, if any.
func signedIn(r *http.Request) (staffSession, bool) {
	s, ok := r.Context().Value(sessionKey{}).(staffSession)
	return s, ok
}

// open says whether a path is served without a session.
func open(path string) bool {
	switch path {
	case "/login", "/favicon.ico", "/favicon.svg", "/about.json":
		return true
	}
	return false
}

// requireLogin serves the open paths as they are and every other with the
// session the request's cookie names, or sends the browser to the login
// page with the page it wanted to come back to. An HTMX request that has
// lost its session is told to load the login page whole.
func (s *Site) requireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if open(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if sess, ok := s.lookup(r); ok {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey{}, sess)))
			return
		}
		target := s.cfg.Scope + "login"
		if r.Method == "GET" {
			if back := strings.TrimPrefix(r.URL.RequestURI(), "/"); inScope(back) && back != "" {
				target += "?redirect=" + url.QueryEscape(back)
			}
		}
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("HX-Redirect", target)
			http.Error(w, "Sign in to continue", http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
	})
}

// lookup is the session the request's cookie names, when the store knows
// it and the user it names may use the staff web.
func (s *Site) lookup(r *http.Request) (staffSession, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return staffSession{}, false
	}
	userID, ok := s.sessions.Lookup(c.Value)
	if !ok {
		return staffSession{}, false
	}
	user, err := s.cfg.Users.User(r.Context(), userID)
	if err != nil {
		if !errors.Is(err, core.ErrNotFound) {
			log.Printf("staff: session's user %s: %v", userID, err)
		}
		return staffSession{}, false
	}
	role, ok := RoleOf(user)
	if !ok {
		return staffSession{}, false
	}
	return staffSession{user: user, role: role, token: c.Value}, true
}

func (s *Site) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: s.cfg.Scope, HttpOnly: true,
		Secure: s.cfg.SecureCookies, SameSite: http.SameSiteStrictMode,
	})
}

func (s *Site) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: s.cfg.Scope, HttpOnly: true, MaxAge: -1})
}
