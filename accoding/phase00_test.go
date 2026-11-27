package accoding

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestReadRetryOnlyTransient(t *testing.T) {
	calls := 0
	c := newClient(transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("temporary network failure")
		}
		if calls == 2 {
			return response(r, 503, "temporary"), nil
		}
		return js(r, `{"id":7}`), nil
	}))
	if u, err := c.Account(context.Background()); err != nil || u.ID != "7" || calls != 3 {
		t.Fatal(u, err, calls)
	}
}
func TestReadDoesNotRetryRateOrAuth(t *testing.T) {
	for _, tc := range []struct {
		status int
		kind   Kind
	}{{429, Rate}, {401, Auth}, {403, Forbidden}, {404, Protocol}} {
		calls := 0
		c := newClient(transportFunc(func(r *http.Request) (*http.Response, error) { calls++; return response(r, tc.status, ""), nil }))
		if _, err := c.Account(context.Background()); !IsKind(err, tc.kind) || calls != 1 {
			t.Fatal(tc, err, calls)
		}
	}
}
func TestRetryCancellation(t *testing.T) {
	calls := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := newClient(transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		time.AfterFunc(25*time.Millisecond, cancel)
		return response(r, 503, ""), nil
	}))
	if _, err := c.Account(ctx); !IsKind(err, Cancelled) || calls != 1 {
		t.Fatal(err, calls)
	}
}
func TestIncompleteSamplesNotMispaired(t *testing.T) {
	for _, md := range []string{
		"## 输入样例 1\n```\n1\n```\n## 输入样例 2\n```\n2\n```\n## 输出样例 2\n```\n4\n```\n",
		"## 输入样例\n```\n1\n```\n## 输出样例\n```\n2\n",
	} {
		if s, w := samplesMarkdown(md); len(s) != 0 || len(w) == 0 {
			t.Fatal(s, w)
		}
	}
}
