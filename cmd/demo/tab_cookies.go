package main

import (
	"net/http"
	"sync"
)

// keepCookiesInTab wraps a handler whose cookies the browser cannot keep:
// a service worker's response is synthetic, and the browser drops its
// Set-Cookie. The wrapper keeps every cookie the handler sets and presents
// them on each request that carries none, as the browser would have; a
// cookie the handler clears (MaxAge < 0) is forgotten. The tab holds the
// whole bank for one person, so one jar is the right number.
func keepCookiesInTab(next http.Handler) http.Handler {
	jar := &tabCookies{next: next, cookies: map[string]string{}}
	return jar
}

type tabCookies struct {
	next    http.Handler
	mu      sync.Mutex
	cookies map[string]string
}

func (j *tabCookies) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Cookie") == "" {
		j.mu.Lock()
		for name, value := range j.cookies {
			r.AddCookie(&http.Cookie{Name: name, Value: value})
		}
		j.mu.Unlock()
	}
	c := &cookieCatcher{ResponseWriter: w, jar: j}
	j.next.ServeHTTP(c, r)
	if !c.wroteHeader { // a handler that wrote nothing still set its header
		j.keep(c.Header().Values("Set-Cookie"))
	}
}

// cookieCatcher reads the cookies a response sets as its header goes out.
type cookieCatcher struct {
	http.ResponseWriter
	jar         *tabCookies
	wroteHeader bool
}

func (c *cookieCatcher) WriteHeader(code int) {
	if !c.wroteHeader {
		c.wroteHeader = true
		c.jar.keep(c.Header().Values("Set-Cookie"))
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *cookieCatcher) Write(b []byte) (int, error) {
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	return c.ResponseWriter.Write(b)
}

func (j *tabCookies) keep(setCookies []string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, line := range setCookies {
		c, err := http.ParseSetCookie(line)
		if err != nil {
			continue
		}
		if c.MaxAge < 0 {
			delete(j.cookies, c.Name)
			continue
		}
		j.cookies[c.Name] = c.Value
	}
}
