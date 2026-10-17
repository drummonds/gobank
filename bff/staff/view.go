package staff

import (
	"net/http"

	"git.bytestone.uk/hum3/gobank/internal/view"
)

// The staff web shows balances in the bank's view by default (ADR-0008:
// engineers and staff work in the accounting view) and lets a session
// switch to the customer's. The choice is a cookie, so it outlives a
// restart and binds to nothing in the bank.

const viewCookie = "staff_view"

// view is the view the request's session has chosen.
func (s *Site) view(r *http.Request) view.View {
	c, err := r.Cookie(viewCookie)
	if err != nil {
		return view.Bank
	}
	return view.Parse(c.Value)
}

// viewSwitch switches this browser between the bank's view and the
// customer's: GET view/{name}?back=<path> sets the staff_view cookie for a
// year and returns to the page that offered it.
func (s *Site) viewSwitch(w http.ResponseWriter, r *http.Request) {
	v := view.Parse(r.PathValue("name"))
	http.SetCookie(w, &http.Cookie{
		Name: viewCookie, Value: string(v), Path: s.cfg.Scope, HttpOnly: true,
		Secure: s.cfg.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: 365 * 24 * 3600,
	})
	s.redirect(w, r, r.URL.Query().Get("back"))
}
