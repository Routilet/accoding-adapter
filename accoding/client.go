// Package accoding implements the Accoding platform's online business operations.
package accoding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const StudentOrigin = "https://accoding.buaa.edu.cn"
const AdminOrigin = StudentOrigin + ":4000"
const responseLimit = 8 << 20

type Kind string

const (
	Auth      Kind = "auth"
	Forbidden Kind = "forbidden"
	Rate      Kind = "rate"
	Server    Kind = "server"
	Protocol  Kind = "protocol"
	Network   Kind = "network"
	Unknown   Kind = "unknown"
	Cancelled Kind = "cancelled"
)

type Error struct {
	Kind    Kind
	Message string
}

func (e *Error) Error() string      { return e.Message }
func fail(k Kind, m string) error   { return &Error{k, m} }
func IsKind(err error, k Kind) bool { var e *Error; return errors.As(err, &e) && e.Kind == k }

var digits = regexp.MustCompile(`^[0-9]+$`)

func validID(id string) bool { return len(id) <= 64 && digits.MatchString(id) }
func checkID(id string) error {
	if !validID(id) {
		return fail(Protocol, "无效的远程 ID")
	}
	return nil
}

type Client struct {
	transport  http.RoundTripper
	student    *sessionJar
	mu         sync.Mutex
	account    User
	generation uint64
	session    context.Context
	cancel     context.CancelFunc
	submitMu   sync.Mutex
}
type endpoint struct {
	origin     string
	jar        http.CookieJar
	transport  http.RoundTripper
	adminWrite string
}
type reply struct {
	url    *url.URL
	body   []byte
	header http.Header
}

func New() *Client { return newClient(http.DefaultTransport) }
func newClient(t http.RoundTripper) *Client {
	j := newSessionJar()
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{transport: t, student: j, session: ctx, cancel: cancel}
}
func (c *Client) endpoint(admin bool, write string) endpoint {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.endpointLocked(admin, write)
}
func (c *Client) endpointLocked(admin bool, write string) endpoint {
	if !admin {
		return endpoint{StudentOrigin, c.student, c.transport, ""}
	}
	j := c.student.clone()
	return endpoint{AdminOrigin, j, c.transport, write}
}
func (c *Client) operation(ctx context.Context, admin bool, write string) (context.Context, endpoint, func()) {
	c.mu.Lock()
	e := c.endpointLocked(admin, write)
	s := c.session
	c.mu.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s, cancel)
	if s.Err() != nil {
		cancel()
	}
	return ctx, e, func() { stop(); cancel() }
}

// Only transient GET failures may be retried. The whole retry sequence is
// bounded; POST (including admin read queries) is always sent at most once.
func (e endpoint) request(ctx context.Context, method, path, body, contentType string, submission bool) (reply, error) {
	if method != http.MethodGet || submission {
		return e.once(ctx, method, path, body, contentType, submission)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for attempt := 0; ; attempt++ {
		r, err := e.once(ctx, method, path, body, contentType, false)
		if err == nil || attempt >= 2 || ctx.Err() != nil || (!IsKind(err, Network) && !IsKind(err, Server)) {
			return r, err
		}
		timer := time.NewTimer(time.Duration(1<<attempt) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return reply{}, fail(Cancelled, "请求取消或超时")
		case <-timer.C:
		}
	}
}

func (e endpoint) once(ctx context.Context, method, path, body, contentType string, submission bool) (reply, error) {
	if err := ctx.Err(); err != nil {
		return reply{}, fail(Cancelled, "操作已取消")
	}
	u0, _ := url.Parse(e.origin)
	u, err := u0.Parse(path)
	if err != nil {
		return reply{}, fail(Protocol, "无效请求地址")
	}
	if !sameOrigin(u, u0) {
		return reply{}, fail(Protocol, "拒绝跨域请求")
	}
	if e.origin == AdminOrigin && method == "POST" {
		if u.RawQuery != "" || u.Fragment != "" {
			return reply{}, fail(Forbidden, "管理端 POST 地址非法")
		}
		if u.Path == "/submission/getSubmissionApi" {
			f, err := url.ParseQuery(body)
			var ids []string
			if err != nil || len(f) != 1 || len(f["submission_id"]) != 1 || json.Unmarshal([]byte(f.Get("submission_id")), &ids) != nil || len(ids) > 1000 || len(ids) == 0 {
				return reply{}, fail(Forbidden, "非法管理端查询")
			}
			for _, id := range ids {
				if !validID(id) {
					return reply{}, fail(Forbidden, "非法管理端查询")
				}
			}
		} else if e.adminWrite == "" || u.Path != e.adminWrite {
			return reply{}, fail(Forbidden, "管理端仅允许指定题目的提交")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client := &http.Client{Transport: e.transport, Jar: e.jar, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 6 || !sameOrigin(req.URL, u0) || req.Method != "GET" {
			return http.ErrUseLastResponse
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), strings.NewReader(body))
	if err != nil {
		return reply{}, fail(Protocol, "无法构造请求")
	}
	req.Header.Set("Accept", "application/json, text/html;q=0.9")
	if method == "POST" {
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Origin", e.origin)
		req.Header.Set("Referer", u.String())
		req.GetBody = nil
	}
	// No automatic write retry, including redirects preserving POST.
	res, err := client.Do(req)
	if err != nil {
		if submission {
			return reply{}, fail(Unknown, "提交结果不确定，请查询记录，勿直接重发")
		}
		if ctx.Err() != nil {
			return reply{}, fail(Cancelled, "请求取消或超时")
		}
		return reply{}, fail(Network, "网络请求失败")
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 && res.StatusCode < 400 {
		if submission {
			return reply{}, fail(Unknown, "提交重定向未能安全确认")
		}
		return reply{}, fail(Protocol, "拒绝重定向")
	}
	if res.StatusCode == 401 {
		return reply{}, fail(Auth, "登录已失效")
	}
	if res.StatusCode == 403 {
		return reply{}, fail(Forbidden, "无访问权限或 CSRF 校验失败")
	}
	if res.StatusCode == 429 {
		if submission {
			return reply{}, fail(Unknown, "提交响应限流，结果待确认")
		}
		return reply{}, fail(Rate, "请求过于频繁")
	}
	if res.StatusCode >= 500 {
		if submission {
			return reply{}, fail(Unknown, "服务器异常，提交结果待确认")
		}
		return reply{}, fail(Server, "服务暂时不可用")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		if submission {
			return reply{}, fail(Unknown, "提交未能确认")
		}
		return reply{}, fail(Protocol, fmt.Sprintf("HTTP %d", res.StatusCode))
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, responseLimit+1))
	if err != nil || len(b) > responseLimit {
		if submission {
			return reply{}, fail(Unknown, "提交响应不完整，结果待确认")
		}
		return reply{}, fail(Protocol, "响应不完整或超过限制")
	}
	return reply{res.Request.URL, b, res.Header}, nil
}
func sameOrigin(u, base *url.URL) bool {
	return u.Scheme == base.Scheme && u.Host == base.Host && u.User == nil && u.Fragment == ""
}
func (e endpoint) json(ctx context.Context, method, path, body, ct string, submission bool, out any) error {
	r, err := e.request(ctx, method, path, body, ct, submission)
	if err != nil {
		return err
	}
	if r.url.Path == "/user/login" || path == "/api/users/me" && len(strings.TrimSpace(string(r.body))) == 0 {
		if submission {
			return fail(Unknown, "提交后登录失效，结果待确认")
		}
		return fail(Auth, "请先登录")
	}
	if json.Unmarshal(r.body, out) != nil {
		if submission {
			return fail(Unknown, "提交响应无法识别，结果待确认")
		}
		return fail(Protocol, "服务端未返回预期 JSON")
	}
	return nil
}
