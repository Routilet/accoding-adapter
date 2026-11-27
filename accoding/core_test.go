package accoding

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(r *http.Request, status int, body string, headers ...string) *http.Response {
	h := http.Header{}
	for i := 0; i < len(headers); i += 2 {
		h.Add(headers[i], headers[i+1])
	}
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}
func js(r *http.Request, s string) *http.Response {
	return response(r, 200, s, "Content-Type", "application/json")
}

const contestFixture = `{"id":42,"title":"作业","problems":[{"id":"20","title":"B","description":"## 输出样例\n~~~\n2\n~~~\n","test_setting":"{\"supported_languages\":\"C99,C++17\"}","contest_problem_list":{"order":8}},{"id":10,"title":"A","description":"## 输入样例 $1$\n~~~\n 1\n~~~\n## 输出样例\n~~~\n2\n~~~\n","test_setting":"{\"supported_languages\":\"C99\",\"time_limit\":1000}","contest_problem_list":{"order":5}}]}`
const pageFixture = `<!DOCTYPE html><html><body><div class="markdown-body"><h1 class="problem-title">加法</h1><h2>输入样例</h2><pre>1 &lt; 2
</pre><h2>输出样例</h2><pre>yes
</pre><script>bad()</script><form><input name="secret" value="NOPE"/></form></div><form method="post" action="/problem/10/submit"><input name="_csrf" value="csrf"><select name="lang"><option value="C99">C99</option></select></form></body></html>`

func TestLoginAndLogout(t *testing.T) {
	c := newClient(transportFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/user/login":
			if r.Method == "GET" {
				return response(r, 200, `<form action="/user/login"><input name="_csrf" value="x"><input name="password"></form>`, "Set-Cookie", "sid=pre; Path=/; Secure"), nil
			}
			b, _ := io.ReadAll(r.Body)
			v, _ := url.ParseQuery(string(b))
			if v.Get("_csrf") != "x" || v.Get("password") != "p" || !strings.Contains(r.Header.Get("Cookie"), "sid=pre") {
				t.Fatal("invalid login form")
			}
			return response(r, 200, "ok", "Set-Cookie", "sid=auth; Path=/; Secure"), nil
		case "/api/users/me":
			if strings.Contains(r.Header.Get("Cookie"), "sid=auth") {
				return js(r, `{"id":7,"nickname":"student"}`), nil
			}
			return js(r, ``), nil
		}
		t.Fatal(r.URL)
		return nil, nil
	}))
	u, e := c.Login(context.Background(), "u", "p")
	if e != nil || u.ID != "7" {
		t.Fatal(u, e)
	}
	c.Logout()
	if _, e = c.Account(context.Background()); !IsKind(e, Auth) {
		t.Fatal(e)
	}
}
func TestContestAndSamples(t *testing.T) {
	c := newClient(transportFunc(func(r *http.Request) (*http.Response, error) { return js(r, contestFixture), nil }))
	v, e := c.GetContest(context.Background(), "42")
	if e != nil || len(v.Problems) != 2 {
		t.Fatal(v, e)
	}
	a, b := v.Problems[0], v.Problems[1]
	if a.Target.ProblemID != "10" || a.order != 0 || a.Label != "A" || a.Samples[0] != (Sample{" 1\n", "2\n"}) || b.Samples[0].Input != "" || a.TimeLimit != "1000" {
		t.Fatal(v)
	}
	if _, e = c.GetContest(context.Background(), "../x"); !IsKind(e, Protocol) {
		t.Fatal(e)
	}
}
func TestProblemPage(t *testing.T) {
	c := newClient(transportFunc(func(r *http.Request) (*http.Response, error) { return response(r, 200, pageFixture), nil }))
	p, e := c.GetProblem(context.Background(), Target{Kind: "problemset", ProblemID: "10"})
	if e != nil || len(p.Samples) != 1 || p.Samples[0].Input != "1 < 2\n" || strings.Contains(p.Statement.Content, "NOPE") || strings.Contains(p.Statement.Content, "bad()") || p.Statement.Format != "text" {
		t.Fatal(p, e)
	}
}
func TestBlockedRedirect(t *testing.T) {
	calls := 0
	c := newClient(transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return response(r, 302, "", "Location", "https://example.org/leak"), nil
	}))
	_, e := c.Account(context.Background())
	if !IsKind(e, Protocol) || calls != 1 {
		t.Fatal(e, calls)
	}
}
func TestAdminReadOnly(t *testing.T) {
	calls := 0
	c := newClient(transportFunc(func(r *http.Request) (*http.Response, error) { calls++; return js(r, `[]`), nil }))
	e := c.endpoint(true, "")
	_, err := e.request(context.Background(), "POST", "/problem/10/submit", "code=x", formCT, true)
	if !IsKind(err, Forbidden) || calls != 0 {
		t.Fatal(err, calls)
	}
	_, err = e.request(context.Background(), "POST", "/submission/getSubmissionApi", `submission_id=%5B%221%22%5D`, formCT, false)
	if err != nil || calls != 1 {
		t.Fatal(err, calls)
	}
}
func TestAdminCookieIsolation(t *testing.T) {
	c := newClient(transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Port() == "4000" {
			return response(r, 200, `{"id":7}`, "Set-Cookie", "sid=admin; Path=/; Secure"), nil
		}
		if strings.Contains(r.Header.Get("Cookie"), "sid=admin") {
			t.Fatal("admin overwrote student cookie")
		}
		return js(r, `{"id":7}`), nil
	}))
	u, _ := url.Parse(StudentOrigin)
	c.student.SetCookies(u, []*http.Cookie{{Name: "sid", Value: "student", Path: "/", Secure: true}})
	if _, e := currentUser(context.Background(), c.endpoint(true, "")); e != nil {
		t.Fatal(e)
	}
	if _, e := c.Account(context.Background()); e != nil {
		t.Fatal(e)
	}
}
func TestSubmissionUnknownAndJournal(t *testing.T) {
	posts := 0
	c := newClient(transportFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/users/me":
			return js(r, `{"id":7}`), nil
		case "/api/contests/42":
			return js(r, contestFixture), nil
		case "/api/contests/42/submissions":
			posts++
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload["order"] != float64(0) || payload["code"] != "int main(){}" {
				t.Fatal(payload)
			}
			return nil, errors.New("connection lost")
		default:
			t.Fatal(r.URL)
			return nil, nil
		}
	}))
	j := FileJournal{t.TempDir()}
	target := Target{Kind: "contest", ContestID: "42", ProblemID: "10"}
	for range 2 {
		_, e := c.Submit(context.Background(), target, "7", "C99", "int main(){}", j)
		if !IsKind(e, Unknown) {
			t.Fatal(e)
		}
	}
	if posts != 1 {
		t.Fatal("duplicate POST", posts)
	}
}
func TestSubmissionSuccess(t *testing.T) {
	posts := 0
	c := newClient(transportFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/users/me":
			return js(r, `{"id":7}`), nil
		case "/api/contests/42":
			return js(r, contestFixture), nil
		case "/api/contests/42/submissions":
			posts++
			return js(r, `{"id":123,"result":"WT","problem_id":10,"creator_id":7,"lang":"C99"}`), nil
		}
		t.Fatal(r.URL)
		return nil, nil
	}))
	j := FileJournal{t.TempDir()}
	target := Target{Kind: "contest", ContestID: "42", ProblemID: "10"}
	for range 2 {
		s, e := c.Submit(context.Background(), target, "7", "C99", "int main(){}", j)
		if e != nil || s.ID != "123" || !s.Pending() {
			t.Fatal(s, e)
		}
	}
	if posts != 2 {
		t.Fatal(posts)
	}
}
func TestProblemsetLostResponseRecovery(t *testing.T) {
	posts := 0
	code := "int main(){return 0;}"
	c := newClient(transportFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/users/me":
			return js(r, `{"id":7}`), nil
		case "/problem/10/index":
			return response(r, 200, pageFixture), nil
		case "/problem/10/submit":
			posts++
			return nil, errors.New("lost")
		case "/problem/10/submission":
			if posts == 0 {
				return response(r, 200, `<table></table>`), nil
			}
			return response(r, 200, `<table><tr><td id="submission_id1">123</td><td><a href="/user/7/index">u</a></td></tr></table>`), nil
		case "/submission/getSubmissionApi":
			return js(r, `[{"id":"123","result":"AC","problem_id":10,"creator_id":7,"lang":"C99"}]`), nil
		case "/submission/123":
			return response(r, 200, "<pre><code>/* Submission_id: 123 Problem: 10 */\n\n"+code+"</code></pre>"), nil
		}
		t.Fatal(r.URL)
		return nil, nil
	}))
	s, e := c.Submit(context.Background(), Target{Kind: "problemset", ProblemID: "10"}, "7", "C99", code, FileJournal{t.TempDir()})
	if e != nil || s.ID != "123" || posts != 1 {
		t.Fatal(s, e, posts)
	}
}
func TestJournalGuardsAlternateRoute(t *testing.T) {
	j := FileJournal{t.TempDir()}
	a := Attempt{Account: "7", Target: Target{Kind: "contest", ContestID: "42", ProblemID: "10"}, Language: "C99", SourceHash: sourceHash("code"), CreatedAt: time.Now().UTC()}
	k, e := j.Begin(context.Background(), a)
	if e != nil {
		t.Fatal(e)
	}
	b := a
	b.Target = Target{Kind: "problemset", ProblemID: "10", Service: "admin"}
	if _, e = j.Begin(context.Background(), b); !IsKind(e, Unknown) {
		t.Fatal(e)
	}
	if e = j.Resolve(context.Background(), k, Submission{ID: "1", Result: "AC", ProblemID: "10", CreatorID: "7"}); e != nil {
		t.Fatal(e)
	}
	if _, e = j.Begin(context.Background(), b); e != nil {
		t.Fatal(e)
	}
}
func TestSampleEmptyAndLiteral(t *testing.T) {
	for _, tc := range []struct{ md, input string }{{"## 输入样例\n无\n## 输出样例\n```\n1\n```\n", ""}, {"## 输入样例\n```\n无\n```\n## 输出样例\n```\n1\n```\n", "无\n"}} {
		s, w := samplesMarkdown(tc.md)
		if len(s) != 1 || s[0].Input != tc.input || len(w) != 0 {
			t.Fatal(s, w)
		}
	}
}
func TestCancelledBeforeNetwork(t *testing.T) {
	c := newClient(transportFunc(func(r *http.Request) (*http.Response, error) { t.Fatal("unexpected network"); return nil, nil }))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := c.Account(ctx); !IsKind(e, Cancelled) {
		t.Fatal(e)
	}
}

func TestCookieCloneRetainsScope(t *testing.T) {
	j := newSessionJar()
	u, _ := url.Parse(StudentOrigin + "/api/users/me")
	j.SetCookies(u, []*http.Cookie{{Name: "api", Value: "1", Path: "/api/", Secure: true}})
	n := j.clone()
	q, _ := url.Parse(AdminOrigin + "/api/users/me")
	if len(n.Cookies(q)) != 1 {
		t.Fatal("missing scoped cookie")
	}
	q, _ = url.Parse(AdminOrigin + "/problem/10/index")
	if len(n.Cookies(q)) != 0 {
		t.Fatal("cookie scope broadened")
	}
}
func TestPost307NeverReplays(t *testing.T) {
	posts := 0
	c := newClient(transportFunc(func(r *http.Request) (*http.Response, error) {
		posts++
		return response(r, 307, "", "Location", "/problem/10/submit"), nil
	}))
	_, e := c.endpoint(false, "").request(context.Background(), "POST", "/problem/10/submit", "code=x", formCT, true)
	if !IsKind(e, Unknown) || posts != 1 {
		t.Fatal(e, posts)
	}
}
func TestInvalidLoginAction(t *testing.T) {
	calls := 0
	c := newClient(transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return response(r, 200, `<form action="https://example.org/collect"><input name="_csrf" value="x"><input name="password"></form>`), nil
	}))
	_, e := c.Login(context.Background(), "user", "pass")
	if !IsKind(e, Protocol) || calls != 1 {
		t.Fatal(e, calls)
	}
}
