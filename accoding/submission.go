package accoding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Prepared struct {
	Target    Target
	Account   ID
	Languages []string
	Warning   string
}

// Prepare does not send code. A contest may require explicit selection of a
// problemset target; it is never silently switched during a submission.
func (c *Client) Prepare(ctx context.Context, t Target) (Prepared, error) {
	if err := t.validate(); err != nil {
		return Prepared{}, err
	}
	ctx, e, done := c.operation(ctx, t.Service == "admin", "")
	defer done()
	return prepare(ctx, e, t)
}
func prepare(ctx context.Context, e endpoint, t Target) (Prepared, error) {
	u, err := currentUser(ctx, e)
	if err != nil {
		return Prepared{}, err
	}
	if t.Kind == "contest" {
		v, err := getContest(ctx, e, t.ContestID)
		if err != nil {
			return Prepared{}, err
		}
		if !v.Active(time.Now()) {
			return Prepared{}, fail(Forbidden, "比赛未开始或已结束；请明确选择有权限的题库目标")
		}
		for _, p := range v.Problems {
			if p.Target.ProblemID == t.ProblemID {
				return Prepared{Target: t, Account: u.ID, Languages: p.Languages}, nil
			}
		}
		return Prepared{}, fail(Forbidden, "比赛中无此题或没有访问权限")
	}
	p, err := getPage(ctx, e, t)
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{Target: t, Account: u.ID, Languages: p.problem.Languages}, nil
}

type Attempt struct {
	Account    ID        `json:"account"`
	Target     Target    `json:"target"`
	Language   string    `json:"language"`
	SourceHash string    `json:"sourceHash"`
	CreatedAt  time.Time `json:"createdAt"`
}

// Journal must durably record Begin before network transmission and reject
// unresolved attempts for the same account and problem (including alternate
// service routes). Resolve records a positively identified submission; Unknown
// leaves the guard in place. If persistence fails, the caller must not retry.
// This storage boundary is intentionally independent of any Companion UI.
type Journal interface {
	Begin(context.Context, Attempt) (string, error)
	Resolve(context.Context, string, Submission) error
	Unknown(context.Context, string) error
}

func sourceHash(s string) string { d := sha256.Sum256([]byte(s)); return hex.EncodeToString(d[:]) }
func (c *Client) Submit(ctx context.Context, t Target, account ID, language, code string, j Journal) (Submission, error) {
	if err := t.validate(); err != nil {
		return Submission{}, err
	}
	if j == nil {
		return Submission{}, fail(Protocol, "提交需要持久化记录保护")
	}
	if !validID(string(account)) || account == "0" || len(code) == 0 || len(code) > 4<<20 || language == "" {
		return Submission{}, fail(Protocol, "提交参数无效")
	}
	c.submitMu.Lock()
	defer c.submitMu.Unlock()
	write := ""
	if t.Service == "admin" {
		write = "/problem/" + t.ProblemID + "/submit"
	}
	ctx, e, done := c.operation(ctx, t.Service == "admin", write)
	defer done()
	u, err := currentUser(ctx, e)
	if err != nil {
		return Submission{}, err
	}
	if u.ID != account {
		return Submission{}, fail(Auth, "账号已变化，请重新确认")
	}
	var action, body, ct string
	var before map[ID]bool
	if t.Kind == "contest" {
		v, err := getContest(ctx, e, t.ContestID)
		if err != nil {
			return Submission{}, err
		}
		if !v.Active(time.Now()) {
			return Submission{}, fail(Forbidden, "比赛未开始或已结束；请重新选择目标")
		}
		var p *Problem
		for i := range v.Problems {
			if v.Problems[i].Target.ProblemID == t.ProblemID {
				p = &v.Problems[i]
				break
			}
		}
		if p == nil {
			return Submission{}, fail(Forbidden, "比赛中无此题")
		}
		if !includes(p.Languages, language) {
			return Submission{}, fail(Protocol, "语言已失效")
		}
		action = "/api/contests/" + t.ContestID + "/submissions"
		b, _ := json.Marshal(map[string]any{"code": code, "lang": language, "order": p.order})
		body = string(b)
		ct = jsonCT
	} else {
		p, err := getPage(ctx, e, t)
		if err != nil {
			return Submission{}, err
		}
		if !includes(p.problem.Languages, language) {
			return Submission{}, fail(Protocol, "语言已失效")
		}
		records, err := list(ctx, e, t, account)
		if err != nil {
			return Submission{}, err
		}
		before = map[ID]bool{}
		for _, s := range records {
			before[s.ID] = true
		}
		v := url.Values{"code": {encodeCode(code, p.shift)}, "lang": {language}}
		if p.csrf != "" {
			v.Set("_csrf", p.csrf)
		}
		action = p.action
		body = v.Encode()
		ct = formCT
	}
	// Revalidate identity after preflight; no write is attempted if the account changed.
	u, err = currentUser(ctx, e)
	if err != nil {
		return Submission{}, err
	}
	if u.ID != account {
		return Submission{}, fail(Auth, "账号已变化，请重新确认")
	}
	if ctx.Err() != nil {
		return Submission{}, fail(Cancelled, "提交已取消")
	}
	key, err := j.Begin(ctx, Attempt{Account: account, Target: t, Language: language, SourceHash: sourceHash(code), CreatedAt: time.Now().UTC()})
	if err != nil {
		return Submission{}, err
	}
	r, err := e.request(ctx, "POST", action, body, ct, true)
	var s Submission
	confirmed := false
	if err == nil && strings.Contains(r.header.Get("Content-Type"), "application/json") && json.Unmarshal(r.body, &s) == nil && s.verify(t, account) == nil && s.CreatorID == account && string(s.ProblemID) == t.ProblemID && s.Lang == language && !before[s.ID] {
		confirmed = true
	}
	// Contest normally responds with JSON. Problemset redirect IDs are verified
	// by read-only query and source matching; a redirect alone is not proof.
	if !confirmed && t.Kind == "problemset" && ctx.Err() == nil {
		for round := 0; round < 3 && !confirmed; round++ {
			if round > 0 {
				select {
				case <-ctx.Done():
					break
				case <-time.After(time.Second):
				}
			}
			if ctx.Err() != nil {
				break
			}
			records, e2 := list(ctx, e, t, account)
			if e2 != nil {
				break
			}
			var matches []Submission
			for i, v := range records {
				if i >= 20 {
					break
				}
				if before[v.ID] || v.CreatorID != account || v.Lang != language || string(v.ProblemID) != t.ProblemID {
					continue
				}
				src, e2 := readSource(ctx, e, t, string(v.ID))
				if e2 == nil && normalizeLF(src) == normalizeLF(code) {
					matches = append(matches, v)
				}
			}
			if len(matches) > 1 {
				break
			}
			if len(matches) == 1 {
				s = matches[0]
				confirmed = true
			}
		}
	}
	// Persistence must survive cancellation of the request context.
	saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if confirmed {
		if err := j.Resolve(saveCtx, key, s); err != nil {
			return s, fail(Unknown, "已获得提交 ID，但记录保存失败；勿重发")
		}
		return s, nil
	}
	if err := j.Unknown(saveCtx, key); err != nil {
		return Submission{}, fail(Unknown, "提交结果不确定且记录更新失败；勿重发")
	}
	return Submission{}, fail(Unknown, "提交结果不确定，请查询记录，勿直接重发")
}
func normalizeLF(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

var sourceHeader = regexp.MustCompile(`(?s)^/\*.*?\*/\r?\n\r?\n`)
var submissionHeader = regexp.MustCompile(`\bSubmission_id:\s*([0-9]+)\b`)
var problemHeader = regexp.MustCompile(`\bProblem(?:_id)?:\s*([0-9]+)\b`)

func readSource(ctx context.Context, e endpoint, t Target, id string) (string, error) {
	if err := checkID(id); err != nil {
		return "", err
	}
	path := "/submission/" + id
	r, err := e.request(ctx, "GET", path, "", "", false)
	if err != nil {
		return "", err
	}
	if r.url.Path != path {
		return "", fail(Protocol, "源码详情路径不匹配")
	}
	n, err := parseHTML(string(r.body))
	if err != nil {
		return "", err
	}
	p := n.one(func(x *node) bool { return x.tag == "code" && x.parent != nil && x.parent.tag == "pre" })
	if p == nil {
		return "", fail(Protocol, "源码详情缺失")
	}
	text := p.text()
	h := sourceHeader.FindString(text)
	if h == "" || capture(submissionHeader, h) != id || capture(problemHeader, h) != t.ProblemID {
		return "", fail(Protocol, "源码头身份不匹配")
	}
	return text[len(h):], nil
}
func query(ctx context.Context, e endpoint, t Target, ids []string, account ID) ([]Submission, error) {
	if len(ids) == 0 {
		return []Submission{}, nil
	}
	var all []Submission
	for i := 0; i < len(ids); i += 100 {
		end := i + 100
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[i:end]
		b, _ := json.Marshal(batch)
		q := url.Values{"submission_id": {string(b)}}.Encode()
		var records []Submission
		var err error
		if e.origin == AdminOrigin {
			err = e.json(ctx, "POST", "/submission/getSubmissionApi", q, formCT, false, &records)
		} else {
			err = e.json(ctx, "GET", "/submission/getSubmissionApi?"+q, "", "", false, &records)
		}
		if err != nil {
			return nil, err
		}
		for _, s := range records {
			if !includes(batch, string(s.ID)) {
				continue
			}
			if err := s.verify(t, account); err != nil {
				return nil, err
			}
			all = append(all, s)
		}
	}
	return all, nil
}
func list(ctx context.Context, e endpoint, t Target, account ID) ([]Submission, error) {
	if t.Kind == "contest" {
		var records []Submission
		if err := e.json(ctx, "GET", "/api/contests/"+t.ContestID+"/submissions", "", "", false, &records); err != nil {
			return nil, err
		}
		out := []Submission{}
		for _, s := range records {
			if string(s.ProblemID) != t.ProblemID || s.CreatorID != account {
				continue
			}
			if err := s.verify(t, account); err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, nil
	}
	path := "/problem/" + t.ProblemID + "/submission"
	r, err := e.request(ctx, "GET", path, "", "", false)
	if err != nil {
		return nil, err
	}
	n, err := parseHTML(string(r.body))
	if err != nil {
		return nil, err
	}
	if r.url.Path != path || n.one(attr("input", "name", "password")) != nil {
		return nil, fail(Auth, "无法读取题库记录")
	}
	var ids []string
	for _, x := range n.all(func(x *node) bool { return strings.HasPrefix(x.attr["id"], "submission_id") }) {
		row := x.ancestor(tag("tr"))
		if row == nil || row.one(attr("a", "href", "/user/"+string(account)+"/index")) == nil {
			continue
		}
		id := strings.TrimSpace(x.text())
		if validID(id) && !includes(ids, id) {
			ids = append(ids, id)
		}
	}
	return query(ctx, e, t, ids, account)
}
func (c *Client) ListSubmissions(ctx context.Context, t Target) ([]Submission, error) {
	if err := t.validate(); err != nil {
		return nil, err
	}
	ctx, e, done := c.operation(ctx, t.Service == "admin", "")
	defer done()
	u, err := currentUser(ctx, e)
	if err != nil {
		return nil, err
	}
	return list(ctx, e, t, u.ID)
}
func (c *Client) GetSubmission(ctx context.Context, t Target, id string) (Submission, error) {
	if err := t.validate(); err != nil {
		return Submission{}, err
	}
	if err := checkID(id); err != nil {
		return Submission{}, err
	}
	ctx, e, done := c.operation(ctx, t.Service == "admin", "")
	defer done()
	u, err := currentUser(ctx, e)
	if err != nil {
		return Submission{}, err
	}
	return getSubmission(ctx, e, t, id, u.ID)
}
func getSubmission(ctx context.Context, e endpoint, t Target, id string, account ID) (Submission, error) {
	if t.Kind == "contest" {
		var s Submission
		if err := e.json(ctx, "GET", fmt.Sprintf("/api/contests/%s/submission/%s", t.ContestID, id), "", "", false, &s); err != nil {
			return s, err
		}
		if string(s.ID) != id {
			return Submission{}, fail(Protocol, "提交 ID 不匹配")
		}
		if err := s.verify(t, account); err != nil {
			return Submission{}, err
		}
		return s, nil
	}
	a, err := query(ctx, e, t, []string{id}, account)
	if err != nil {
		return Submission{}, err
	}
	if len(a) != 1 {
		return Submission{}, fail(Protocol, "未找到唯一提交记录")
	}
	return a[0], nil
}

// Monitor performs bounded, cancellable read-only polling. The observer must
// persist each update before returning nil.
func (c *Client) Monitor(ctx context.Context, t Target, id string, update func(Submission) error) (Submission, error) {
	if err := t.validate(); err != nil {
		return Submission{}, err
	}
	if err := checkID(id); err != nil {
		return Submission{}, err
	}
	ctx, e, done := c.operation(ctx, t.Service == "admin", "")
	defer done()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	u, err := currentUser(ctx, e)
	if err != nil {
		return Submission{}, err
	}
	for {
		v, err := getSubmission(ctx, e, t, id, u.ID)
		if err != nil {
			return Submission{}, err
		}
		if ctx.Err() != nil {
			return Submission{}, fail(Cancelled, "查询已取消")
		}
		if update != nil {
			if err := update(v); err != nil {
				return v, err
			}
		}
		if !v.Pending() {
			return v, nil
		}
		select {
		case <-ctx.Done():
			return v, fail(Cancelled, "查询已停止，可稍后刷新")
		case <-time.After(2 * time.Second):
		}
	}
}

// RecoverAttempt performs only reads; it returns a match only when the source,
// account, language, target, and explicit timestamp identify a unique record.
func (c *Client) RecoverAttempt(ctx context.Context, a Attempt) (*Submission, error) {
	if err := a.Target.validate(); err != nil {
		return nil, err
	}
	if a.Target.Kind != "problemset" || a.SourceHash == "" || a.CreatedAt.IsZero() {
		return nil, fail(Protocol, "当前仅支持题库的不确定提交恢复")
	}
	ctx, e, done := c.operation(ctx, a.Target.Service == "admin", "")
	defer done()
	u, err := currentUser(ctx, e)
	if err != nil {
		return nil, err
	}
	if u.ID != a.Account {
		return nil, fail(Auth, "账号不匹配")
	}
	records, err := list(ctx, e, a.Target, a.Account)
	if err != nil {
		return nil, err
	}
	var matches []Submission
	for _, s := range records {
		when, err := time.Parse(time.RFC3339, s.CreatedAt)
		if err != nil || when.Before(a.CreatedAt.Add(-5*time.Second)) || when.After(a.CreatedAt.Add(120*time.Second)) || s.CreatorID != a.Account || s.Lang != a.Language || string(s.ProblemID) != a.Target.ProblemID {
			continue
		}
		src, err := readSource(ctx, e, a.Target, string(s.ID))
		if err != nil {
			return nil, err
		}
		if a.SourceHash == sourceHash(src) || a.SourceHash == sourceHash(normalizeLF(src)) || a.SourceHash == sourceHash(strings.ReplaceAll(normalizeLF(src), "\n", "\r\n")) {
			matches = append(matches, s)
		}
	}
	if len(matches) != 1 {
		return nil, nil
	}
	return &matches[0], nil
}
