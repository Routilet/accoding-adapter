package accoding

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"time"
)

// Retain original Set-Cookie scope for isolated admin snapshots; reading a
// cookiejar via Cookies() loses Path, Domain, and expiry metadata.
type cookieEvent struct {
	url    url.URL
	cookie http.Cookie
}
type sessionJar struct {
	mu     sync.Mutex
	jar    *cookiejar.Jar
	events []cookieEvent
}

func newSessionJar() *sessionJar { j, _ := cookiejar.New(nil); return &sessionJar{jar: j} }
func (j *sessionJar) Cookies(u *url.URL) []*http.Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.jar.Cookies(u)
}
func (j *sessionJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.jar.SetCookies(u, cookies)
	for _, c := range cookies {
		v := *c
		if v.MaxAge > 0 {
			v.Expires = time.Now().Add(time.Duration(v.MaxAge) * time.Second)
			v.MaxAge = 0
		}
		j.events = append(j.events, cookieEvent{url: *u, cookie: v})
	}
}
func (j *sessionJar) clone() *sessionJar {
	j.mu.Lock()
	defer j.mu.Unlock()
	n := newSessionJar()
	for _, ev := range j.events {
		u, c := ev.url, ev.cookie
		n.jar.SetCookies(&u, []*http.Cookie{&c})
	}
	return n
}
