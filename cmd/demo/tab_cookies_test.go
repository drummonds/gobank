package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// In the tab the browser drops the cookies a service worker's response
// sets, so the demo keeps them itself: a cookie set on one response is
// presented on the next request, and a cookie cleared is gone.
func TestTabKeepsTheCookiesItIsSet(t *testing.T) {
	var seen []*http.Cookie
	h := keepCookiesInTab(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Cookies()
		switch r.URL.Path {
		case "/login":
			http.SetCookie(w, &http.Cookie{Name: "mb_session", Value: "tok", Path: "/v1"})
			http.Redirect(w, r, "/home", http.StatusSeeOther)
		case "/logout":
			http.SetCookie(w, &http.Cookie{Name: "mb_session", Value: "", Path: "/v1", MaxAge: -1})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	call := func(path string) {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
	}
	call("/home")
	if len(seen) != 0 {
		t.Fatalf("first request should carry no cookie, got %v", seen)
	}
	call("/login")
	call("/home")
	if len(seen) != 1 || seen[0].Name != "mb_session" || seen[0].Value != "tok" {
		t.Fatalf("after login the request should carry the session cookie, got %v", seen)
	}
	call("/logout")
	call("/home")
	if len(seen) != 0 {
		t.Fatalf("after logout the request should carry no cookie, got %v", seen)
	}
}
