package bff

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.bytestone.uk/hum3/gobank/screen"
)

// Config configures a Server. Zero values take the defaults noted.
type Config struct {
	Bank Bank
	Auth Authenticator

	SessionTTL  time.Duration // absolute session lifetime; default 8h
	SessionIdle time.Duration // idle timeout; default 15m

	MaxLoginFailures int           // per customer ID and per client address; default 5
	LoginWindow      time.Duration // sliding window for failures; default 15m

	SecureCookies bool // set Secure on the session cookie (HTML clients); on unless serving plain HTTP
	Logger        *slog.Logger
	Now           func() time.Time
}

// Server is the BFF HTTP handler.
type Server struct {
	cfg      Config
	mux      *http.ServeMux
	sessions *SessionStore
	limiter  *failureLimiter
	log      *slog.Logger
}

const cookieName = "mb_session"

// NewServer builds the handler. Every route except /v1/health, GET
// /v1/screen/login and POST /v1/login requires a valid session.
func NewServer(cfg Config) *Server {
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 8 * time.Hour
	}
	if cfg.SessionIdle == 0 {
		cfg.SessionIdle = 15 * time.Minute
	}
	if cfg.MaxLoginFailures == 0 {
		cfg.MaxLoginFailures = 5
	}
	if cfg.LoginWindow == 0 {
		cfg.LoginWindow = 15 * time.Minute
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &Server{
		cfg:      cfg,
		mux:      http.NewServeMux(),
		sessions: NewSessionStore(cfg.SessionTTL, cfg.SessionIdle, cfg.Now),
		limiter:  newFailureLimiter(cfg.MaxLoginFailures, cfg.LoginWindow, cfg.Now),
		log:      cfg.Logger,
	}
	s.mux.HandleFunc("GET /v1/health", s.health)
	s.mux.HandleFunc("GET "+PathScreenLogin, s.loginScreen)
	s.mux.HandleFunc("POST "+PathLogin, s.login)
	s.mux.HandleFunc("POST "+PathLogout, s.authed(s.logout))
	s.mux.HandleFunc("GET "+PathScreenHome, s.authed(s.accounts))
	s.mux.HandleFunc("GET "+PathScreenTx, s.authed(s.activity))
	s.mux.HandleFunc("GET "+PathScreenProduct+"{index}", s.authed(s.product))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.writeError(w, r, http.StatusNotFound, "not_found", "No such endpoint", nil)
	})
	return s
}

// Sessions exposes the session store, e.g. for a periodic Sweep.
func (s *Server) Sessions() *SessionStore { return s.sessions }

// ServeHTTP applies the security headers every response carries, then routes.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'")
	s.mux.ServeHTTP(w, r)
}

type ctxKey struct{}

type principal struct {
	customerID string
	token      string
}

// authed wraps a handler so it only runs with a valid session. The customer
// ID is placed in the context; handlers never read it from the request.
func (s *Server) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearer(r)
		if token == "" {
			if c, err := r.Cookie(cookieName); err == nil {
				token = c.Value
			}
		}
		custID, ok := s.sessions.Lookup(token)
		if !ok {
			s.log.Info("bff.unauthenticated", "path", r.URL.Path, "ip", clientIP(r))
			if wantsHTML(r) {
				http.Redirect(w, r, PathScreenLogin, http.StatusSeeOther)
				return
			}
			sc := LoginScreen("")
			s.writeError(w, r, http.StatusUnauthorized, "unauthenticated", "Log in to continue", &sc)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKey{}, principal{customerID: custID, token: token})
		next(w, r.WithContext(ctx))
	}
}

func who(r *http.Request) principal {
	p, _ := r.Context().Value(ctxKey{}).(principal)
	return p
}

func bearer(r *http.Request) string {
	const prefix = "Bearer "
	a := r.Header.Get("Authorization")
	if len(a) > len(prefix) && strings.EqualFold(a[:len(prefix)], prefix) {
		return strings.TrimSpace(a[len(prefix):])
	}
	return ""
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// wantsHTML is true for browsers: Accept lists text/html and not JSON.
func wantsHTML(r *http.Request) bool {
	a := r.Header.Get("Accept")
	return strings.Contains(a, "text/html") && !strings.Contains(a, "application/json")
}

// --- handlers ---

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}` + "\n"))
}

func (s *Server) loginScreen(w http.ResponseWriter, r *http.Request) {
	s.writeScreen(w, r, http.StatusOK, LoginScreen(""))
}

type loginRequest struct {
	CustomerID string `json:"customer_id"`
	Password   string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	isForm := false
	switch {
	case strings.HasPrefix(r.Header.Get("Content-Type"), "application/json"):
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			s.writeError(w, r, http.StatusBadRequest, "bad_request", "Malformed login request", nil)
			return
		}
	default:
		isForm = true
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if err := r.ParseForm(); err != nil {
			s.writeError(w, r, http.StatusBadRequest, "bad_request", "Malformed login form", nil)
			return
		}
		req.CustomerID = r.PostFormValue("customer_id")
		req.Password = r.PostFormValue("password")
	}
	req.CustomerID = strings.TrimSpace(req.CustomerID)
	ip := clientIP(r)
	if req.CustomerID == "" || len(req.CustomerID) > 64 || len(req.Password) > 256 {
		s.limiter.fail("ip:" + ip)
		sc := LoginScreen("Enter your customer ID and password.")
		s.writeError(w, r, http.StatusBadRequest, "bad_request", "Enter your customer ID and password.", &sc)
		return
	}

	for _, key := range []string{"cust:" + req.CustomerID, "ip:" + ip} {
		if wait := s.limiter.retryAfter(key); wait > 0 {
			s.log.Warn("bff.login.locked", "customer", req.CustomerID, "ip", ip, "key", key, "retry_after", wait)
			mins := int(wait.Minutes()) + 1
			msg := fmt.Sprintf("Too many attempts. Try again in %d minutes.", mins)
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			sc := LoginScreen(msg)
			s.writeError(w, r, http.StatusTooManyRequests, "rate_limited", msg, &sc)
			return
		}
	}

	cust, err := s.cfg.Auth.Authenticate(r.Context(), req.CustomerID, req.Password)
	if err != nil {
		s.limiter.fail("cust:" + req.CustomerID)
		s.limiter.fail("ip:" + ip)
		s.log.Info("bff.login", "customer", req.CustomerID, "ip", ip, "ok", false)
		if !errors.Is(err, ErrBadCredentials) {
			s.log.Error("bff.login.error", "err", err)
		}
		const msg = "Unknown customer ID or wrong password."
		sc := LoginScreen(msg)
		s.writeError(w, r, http.StatusUnauthorized, "bad_credentials", msg, &sc)
		return
	}
	s.limiter.reset("cust:" + req.CustomerID)
	token, expires := s.sessions.Create(cust.ID)
	s.log.Info("bff.login", "customer", cust.ID, "ip", ip, "ok", true)

	if isForm || wantsHTML(r) {
		http.SetCookie(w, &http.Cookie{
			Name: cookieName, Value: token, Path: "/v1", HttpOnly: true,
			Secure: s.cfg.SecureCookies, SameSite: http.SameSiteStrictMode, Expires: expires,
		})
		http.Redirect(w, r, PathScreenHome, http.StatusSeeOther)
		return
	}
	home, err := s.buildAccounts(r.Context(), cust.ID)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "internal", "Could not load accounts", nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token":      token,
		"expires_at": expires.UTC().Format(time.RFC3339),
		"screen":     home,
	})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	p := who(r)
	s.sessions.Revoke(p.token)
	s.log.Info("bff.logout", "customer", p.customerID)
	if wantsHTML(r) {
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/v1", HttpOnly: true, MaxAge: -1})
		http.Redirect(w, r, PathScreenLogin, http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"screen": LoginScreen("")})
}

func (s *Server) buildAccounts(ctx context.Context, custID string) (screen.Screen, error) {
	cust, err := s.cfg.Bank.Customer(ctx, custID)
	if err != nil {
		return screen.Screen{}, err
	}
	accts, err := s.cfg.Bank.Accounts(ctx, custID)
	if err != nil {
		return screen.Screen{}, err
	}
	return AccountsScreen(cust, accts), nil
}

func (s *Server) accounts(w http.ResponseWriter, r *http.Request) {
	sc, err := s.buildAccounts(r.Context(), who(r).customerID)
	if err != nil {
		s.bankError(w, r, err)
		return
	}
	s.writeScreen(w, r, http.StatusOK, sc)
}

func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	p := who(r)
	cust, err := s.cfg.Bank.Customer(r.Context(), p.customerID)
	if err != nil {
		s.bankError(w, r, err)
		return
	}
	page, err := s.cfg.Bank.Transactions(r.Context(), p.customerID, pageParam(r))
	if err != nil {
		s.bankError(w, r, err)
		return
	}
	s.writeScreen(w, r, http.StatusOK, ActivityScreen(cust, page))
}

func (s *Server) product(w http.ResponseWriter, r *http.Request) {
	p := who(r)
	idx, err := strconv.Atoi(r.PathValue("index"))
	if err != nil || idx < 0 {
		s.writeError(w, r, http.StatusNotFound, "not_found", "No such account", nil)
		return
	}
	cust, err := s.cfg.Bank.Customer(r.Context(), p.customerID)
	if err != nil {
		s.bankError(w, r, err)
		return
	}
	accts, err := s.cfg.Bank.Accounts(r.Context(), p.customerID)
	if err != nil {
		s.bankError(w, r, err)
		return
	}
	if idx >= len(accts) {
		s.writeError(w, r, http.StatusNotFound, "not_found", "No such account", nil)
		return
	}
	page, err := s.cfg.Bank.AccountTransactions(r.Context(), p.customerID, idx, pageParam(r))
	if err != nil {
		s.bankError(w, r, err)
		return
	}
	s.writeScreen(w, r, http.StatusOK, ProductScreen(cust, accts[idx], page))
}

func pageParam(r *http.Request) int {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	return page
}

// --- responses ---

func (s *Server) writeScreen(w http.ResponseWriter, r *http.Request, status int, sc screen.Screen) {
	if wantsHTML(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(screen.Document(sc)))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(sc)
}

// writeError sends an error. When a screen accompanies it (the login screen
// with a notice, say) JSON clients get it under "screen" and browsers get it
// rendered, so a client never has to compose an error view itself.
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, status int, code, message string, sc *screen.Screen) {
	if sc != nil && wantsHTML(r) {
		s.writeScreen(w, r, status, *sc)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := map[string]any{"error": code, "message": message}
	if sc != nil {
		body["screen"] = sc
	}
	_ = json.NewEncoder(w).Encode(body)
}

func (s *Server) bankError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrNotFound) {
		s.writeError(w, r, http.StatusNotFound, "not_found", "Not found", nil)
		return
	}
	s.log.Error("bff.bank", "path", r.URL.Path, "err", err)
	s.writeError(w, r, http.StatusInternalServerError, "internal", "Something went wrong", nil)
}

// Journeys walks every screen the BFF can serve for one customer of bank and
// returns them for diagramming. It is used by `cmd/bff -journeys`.
func Journeys(ctx context.Context, bank Bank, customerID string) (screen.Journeys, error) {
	cust, err := bank.Customer(ctx, customerID)
	if err != nil {
		return screen.Journeys{}, err
	}
	accts, err := bank.Accounts(ctx, customerID)
	if err != nil {
		return screen.Journeys{}, err
	}
	txs, err := bank.Transactions(ctx, customerID, 1)
	if err != nil {
		return screen.Journeys{}, err
	}
	screens := []screen.Screen{LoginScreen(""), AccountsScreen(cust, accts), ActivityScreen(cust, txs)}
	for _, a := range accts {
		page, err := bank.AccountTransactions(ctx, customerID, a.Index, 1)
		if err != nil {
			return screen.Journeys{}, err
		}
		screens = append(screens, ProductScreen(cust, a, page))
	}
	return screen.Journeys{
		Screens:   screens,
		Endpoints: map[string]string{PathLogin: PathScreenHome, PathLogout: PathScreenLogin},
	}, nil
}
