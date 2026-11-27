package accoding

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

const formCT = "application/x-www-form-urlencoded"
const jsonCT = "application/json"

func currentUser(ctx context.Context, e endpoint) (User, error) {
	var u User
	if err := e.json(ctx, "GET", "/api/users/me", "", "", false, &u); err != nil {
		return u, err
	}
	if !validID(string(u.ID)) || u.ID == "0" {
		return User{}, fail(Auth, "请先登录")
	}
	return u, nil
}
func validatedAction(n *node, base *url.URL, path string) (string, error) {
	if n == nil {
		return "", fail(Protocol, "缺少表单")
	}
	a, err := base.Parse(n.attr["action"])
	if err != nil || !sameOrigin(a, base) || a.Path != path || a.RawQuery != "" {
		return "", fail(Protocol, "表单目标发生变化")
	}
	return a.String(), nil
}
func (c *Client) Login(ctx context.Context, username, password string) (User, error) {
	if username == "" || password == "" {
		return User{}, fail(Auth, "账号密码不能为空")
	}
	c.mu.Lock()
	c.generation++
	g := c.generation
	c.mu.Unlock()
	j := newSessionJar()
	e := endpoint{StudentOrigin, j, c.transport, ""}
	r, err := e.request(ctx, "GET", "/user/login", "", "", false)
	if err != nil {
		return User{}, err
	}
	n, err := parseHTML(string(r.body))
	if err != nil {
		return User{}, err
	}
	p := n.one(attr("input", "name", "password"))
	f := p.ancestor(tag("form"))
	a, err := validatedAction(f, r.url, "/user/login")
	if err != nil {
		return User{}, err
	}
	csrf := f.one(attr("input", "name", "_csrf"))
	if csrf == nil || csrf.attr["value"] == "" {
		return User{}, fail(Protocol, "登录表单缺少 CSRF")
	}
	_, err = e.request(ctx, "POST", a, url.Values{"_csrf": {csrf.attr["value"]}, "username": {username}, "password": {password}}.Encode(), formCT, false)
	if err != nil {
		return User{}, err
	}
	u, err := currentUser(ctx, e)
	if err != nil {
		return User{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if g != c.generation || ctx.Err() != nil {
		return User{}, fail(Cancelled, "登录已取消")
	}
	c.cancel()
	c.session, c.cancel = context.WithCancel(context.Background())
	c.student = j
	c.account = u
	return u, nil
}
func (c *Client) Logout() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.cancel()
	c.session, c.cancel = context.WithCancel(context.Background())
	c.student = newSessionJar()
	c.account = User{}
}
func (c *Client) Account(ctx context.Context) (User, error) {
	ctx, e, done := c.operation(ctx, false, "")
	defer done()
	return currentUser(ctx, e)
}

type rawContest struct {
	ID       ID     `json:"id"`
	Title    string `json:"title"`
	Start    string `json:"start_time"`
	End      string `json:"end_time"`
	Problems *[]struct {
		ID          ID      `json:"id"`
		Title       string  `json:"title"`
		Description *string `json:"description"`
		Setting     string  `json:"test_setting"`
		Link        *struct {
			Order *int `json:"order"`
		} `json:"contest_problem_list"`
	} `json:"problems"`
}

func contestLabel(i int) string {
	s := ""
	for i++; i > 0; i = (i - 1) / 26 {
		s = string(rune('A'+(i-1)%26)) + s
	}
	return s
}
func getContest(ctx context.Context, e endpoint, id string) (Contest, error) {
	if err := checkID(id); err != nil {
		return Contest{}, err
	}
	var raw rawContest
	if err := e.json(ctx, "GET", "/api/contests/"+id, "", "", false, &raw); err != nil {
		return Contest{}, err
	}
	if string(raw.ID) != id || raw.Title == "" || raw.Problems == nil {
		return Contest{}, fail(Protocol, "比赛数据不完整")
	}
	a := *raw.Problems
	for _, p := range a {
		if p.Link == nil || p.Link.Order == nil || *p.Link.Order < 0 {
			return Contest{}, fail(Protocol, "题序缺失")
		}
	}
	sort.Slice(a, func(i, j int) bool { return *a[i].Link.Order < *a[j].Link.Order })
	out := Contest{ID: raw.ID, Title: raw.Title, StartTime: raw.Start, EndTime: raw.End}
	seen := map[ID]bool{}
	orders := map[int]bool{}
	for i, p := range a {
		if !validID(string(p.ID)) || seen[p.ID] || orders[*p.Link.Order] || p.Description == nil || p.Title == "" {
			return Contest{}, fail(Protocol, "题目数据不完整或重复")
		}
		seen[p.ID] = true
		orders[*p.Link.Order] = true
		var setting struct {
			Languages string          `json:"supported_languages"`
			Time      json.RawMessage `json:"time_limit"`
			Memory    json.RawMessage `json:"memory_limit"`
			Judge     json.RawMessage `json:"special_judge"`
			Compare   json.RawMessage `json:"special_compared"`
		}
		if json.Unmarshal([]byte(p.Setting), &setting) != nil || len(languages(setting.Languages)) == 0 {
			return Contest{}, fail(Protocol, "评测设置无法识别")
		}
		samples, warnings := samplesMarkdown(*p.Description)
		out.Problems = append(out.Problems, Problem{Target: Target{Kind: "contest", ContestID: id, ProblemID: string(p.ID)}, Title: p.Title, Label: contestLabel(i), Statement: Statement{"markdown", *p.Description, e.origin + "/contest-ng/index.html"}, Languages: languages(setting.Languages), Samples: samples, Warnings: warnings, TimeLimit: scalar(setting.Time), MemoryLimit: scalar(setting.Memory), Special: truthy(setting.Judge) || truthy(setting.Compare), order: i})
	}
	return out, nil
}
func truthy(v json.RawMessage) bool {
	return !includes([]string{"", "null", "false", "0", `"0"`, `""`}, string(v))
}
func (c *Client) GetContest(ctx context.Context, id string) (Contest, error) {
	ctx, e, done := c.operation(ctx, false, "")
	defer done()
	return getContest(ctx, e, id)
}

// ImportContest uses only the authenticated account's existing read permission.
func (c *Client) ImportContest(ctx context.Context, id string) (Contest, error) {
	if err := checkID(id); err != nil {
		return Contest{}, err
	}
	ctx, e, done := c.operation(ctx, true, "")
	defer done()
	probe, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if _, err := currentUser(probe, e); err == nil {
		if v, err := getContest(probe, e, id); err == nil {
			return v, nil
		}
	}
	if ctx.Err() != nil {
		return Contest{}, fail(Cancelled, "导入已取消")
	}
	return c.GetContest(ctx, id)
}

type problemPage struct {
	problem      Problem
	action, csrf string
	shift        bool
}

var timeLimitRE = regexp.MustCompile(`时间限制[:：]\s*([0-9]+\s*\w+)`)
var memoryLimitRE = regexp.MustCompile(`内存限制[:：]\s*([0-9]+\s*\w+)`)
var shiftRE = regexp.MustCompile(`const key\s*=\s*10\s*;`)

func capture(r *regexp.Regexp, s string) string {
	m := r.FindStringSubmatch(s)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}
func getPage(ctx context.Context, e endpoint, t Target) (problemPage, error) {
	if err := t.validate(); err != nil {
		return problemPage{}, err
	}
	if t.Kind != "problemset" || (t.Service == "admin") != (e.origin == AdminOrigin) {
		return problemPage{}, fail(Protocol, "题库服务不匹配")
	}
	path := "/problem/" + t.ProblemID + "/index"
	r, err := e.request(ctx, "GET", path, "", "", false)
	if err != nil {
		return problemPage{}, err
	}
	n, err := parseHTML(string(r.body))
	if err != nil {
		return problemPage{}, err
	}
	if r.url.Path == "/user/login" || n.one(attr("input", "name", "password")) != nil {
		return problemPage{}, fail(Auth, "请先登录")
	}
	h := n.one(func(n *node) bool { return n.tag == "h1" && n.hasClass("problem-title") })
	if r.url.Path != path || h == nil || strings.Contains(string(r.body), "题目不存在，或者你没有权限") {
		return problemPage{}, fail(Forbidden, "题目不存在或无权限")
	}
	b := h.ancestor(func(n *node) bool { return n.hasClass("markdown-body") })
	if b == nil {
		return problemPage{}, fail(Protocol, "题面结构变化")
	}
	sel := n.one(attr("select", "name", "lang"))
	f := sel.ancestor(tag("form"))
	action, err := validatedAction(f, r.url, "/problem/"+t.ProblemID+"/submit")
	if err != nil {
		return problemPage{}, err
	}
	var langs []string
	for _, o := range sel.all(tag("option")) {
		if v := o.attr["value"]; v != "" {
			langs = append(langs, v)
		}
	}
	csrf := ""
	if x := f.one(attr("input", "name", "_csrf")); x != nil {
		csrf = x.attr["value"]
	}
	if len(langs) == 0 || e.origin == StudentOrigin && csrf == "" || e.origin == AdminOrigin && !strings.EqualFold(f.attr["method"], "post") {
		return problemPage{}, fail(Protocol, "提交表单不完整")
	}
	samples, warnings := samplesHTML(b)
	txt := b.text()
	return problemPage{problem: Problem{Target: t, Title: h.text(), Label: "#" + t.ProblemID, Statement: Statement{"text", b.statementText(), r.url.String()}, Languages: langs, Samples: samples, Warnings: warnings, TimeLimit: capture(timeLimitRE, txt), MemoryLimit: capture(memoryLimitRE, txt), Special: strings.Contains(strings.ToLower(txt), "special judge") || strings.Contains(txt, "特殊评测") || strings.Contains(txt, "交互题")}, action: action, csrf: csrf, shift: strings.Contains(n.data, "function encrypt(str, key)") && shiftRE.MatchString(n.data) && strings.Contains(n.data, `str.includes("print")`) && strings.Contains(n.data, `str.includes("length")`)}, nil
}
func (c *Client) GetProblem(ctx context.Context, t Target) (Problem, error) {
	if err := t.validate(); err != nil {
		return Problem{}, err
	}
	ctx, e, done := c.operation(ctx, t.Service == "admin", "")
	defer done()
	if t.Kind == "contest" {
		v, err := getContest(ctx, e, t.ContestID)
		if err != nil {
			return Problem{}, err
		}
		for _, p := range v.Problems {
			if p.Target.ProblemID == t.ProblemID {
				return p, nil
			}
		}
		return Problem{}, fail(Forbidden, "比赛中没有此题")
	}
	p, err := getPage(ctx, e, t)
	return p.problem, err
}
func encodeCode(code string, shift bool) string {
	if !shift || !strings.Contains(code, "print") && !strings.Contains(code, "length") {
		return code
	}
	return strings.Map(func(r rune) rune {
		if r <= 127 {
			return (r + 10) % 128
		}
		return r
	}, code)
}
