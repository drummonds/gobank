package staff

import (
	"errors"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	"git.bytestone.uk/hum3/gobank/core"
)

// The login page (story 1.7.1): a login name and a password, checked
// against the users component; too many failures for one login name or
// from one address lock it for a while, as the customer login does.

// loginRoutes mounts the login page and the logout.
func (s *Site) loginRoutes() {
	s.mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			if _, ok := s.lookup(r); ok {
				s.redirect(w, r, "")
				return
			}
			s.loginPage(w, r, http.StatusOK, "", r.URL.Query().Get("redirect"))
		case "POST":
			s.login(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	s.mux.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			s.redirect(w, r, "")
			return
		}
		if sess, ok := signedIn(r); ok {
			s.sessions.Revoke(sess.token)
			s.pii.revoke(sess.key())
			log.Printf("staff: logout %s", sess.user.Login)
		}
		s.clearSessionCookie(w)
		s.redirect(w, r, "login")
	})
}

func (s *Site) login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Malformed login form", http.StatusBadRequest)
		return
	}
	login := strings.TrimSpace(r.PostFormValue("login"))
	password := r.PostFormValue("password")
	back := r.PostFormValue("redirect")
	ip := clientIP(r)
	if s.cfg.Users == nil {
		s.loginPage(w, r, http.StatusServiceUnavailable, "Staff login is not set up on this deployment.", back)
		return
	}
	if login == "" || len(login) > 100 || len(password) > 256 {
		s.limiter.Fail("ip:" + ip)
		s.loginPage(w, r, http.StatusBadRequest, "Enter your login and password.", back)
		return
	}
	for _, key := range []string{"login:" + login, "ip:" + ip} {
		if wait := s.limiter.RetryAfter(key); wait > 0 {
			log.Printf("staff: login locked %s from %s for %s", login, ip, wait)
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			s.loginPage(w, r, http.StatusTooManyRequests, fmt.Sprintf("Too many attempts. Try again in %d minutes.", int(wait.Minutes())+1), back)
			return
		}
	}
	user, err := s.cfg.Users.AuthenticateUser(r.Context(), login, password)
	role, isStaff := RoleOf(user)
	if err != nil || !isStaff {
		s.limiter.Fail("login:" + login)
		s.limiter.Fail("ip:" + ip)
		if err != nil && !errors.Is(err, core.ErrBadCredentials) {
			log.Printf("staff: login %s: %v", login, err)
		}
		log.Printf("staff: login %s from %s refused", login, ip)
		s.loginPage(w, r, http.StatusUnauthorized, "Unknown login or wrong password.", back)
		return
	}
	s.limiter.Reset("login:" + login)
	token, _ := s.sessions.Create(user.ID)
	s.setSessionCookie(w, token)
	log.Printf("staff: login %s (%s) from %s", user.Login, role, ip)
	s.redirect(w, r, back)
}

// loginPage renders the login form in the layout, with a notice when a
// login was refused and the page to return to afterwards.
func (s *Site) loginPage(w http.ResponseWriter, r *http.Request, status int, notice, back string) {
	if !inScope(back) {
		back = ""
	}
	var b strings.Builder
	b.WriteString(`<div class="columns is-centered"><div class="column is-one-third"><div class="box">`)
	b.WriteString(`<h1 class="title is-4">Staff login</h1>`)
	if notice != "" {
		b.WriteString(`<div class="notification is-danger is-light">` + html.EscapeString(notice) + `</div>`)
	}
	b.WriteString(`<form action="login" method="post" hx-boost="false">`)
	b.WriteString(`<input type="hidden" name="redirect" value="` + html.EscapeString(back) + `">`)
	b.WriteString(`<div class="field"><label class="label" for="login">Login</label><div class="control"><input class="input" type="text" id="login" name="login" autocomplete="username" autofocus required></div></div>`)
	b.WriteString(`<div class="field"><label class="label" for="password">Password</label><div class="control"><input class="input" type="password" id="password" name="password" autocomplete="current-password" required></div></div>`)
	b.WriteString(`<div class="field"><div class="control"><button class="button is-primary" type="submit">Log in</button></div></div>`)
	b.WriteString(`</form>`)
	if s.cfg.LoginNote != "" {
		b.WriteString(`<p class="help">` + html.EscapeString(s.cfg.LoginNote) + `</p>`)
	}
	b.WriteString(`</div></div></div>`)
	w.WriteHeader(status)
	s.render(w, r, b.String(), "Stopped")
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
