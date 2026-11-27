package wire

import (
	"context"
	"encoding/json"
	"github.com/Routilet/accoding-adapter/accoding"
	"net/url"
	"regexp"
	"strings"
)

type importClient interface {
	ImportContest(context.Context, string) (accoding.Contest, error)
	GetProblem(context.Context, accoding.Target) (accoding.Problem, error)
}
type ImportLink struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Service string `json:"service,omitempty"`
}

var problemLink = regexp.MustCompile(`^/problem/([0-9]+)(?:/index)?/?$`)
var contestLink = regexp.MustCompile(`^/contest/([0-9]+)(?:/index|/problem|/edit)?/?$`)
var contestHash = regexp.MustCompile(`^/([0-9]+)(?:/.*)?$`)

func parseLink(s string) (ImportLink, bool) {
	if len(s) > 4096 {
		return ImportLink{}, false
	}
	u, e := url.Parse(strings.TrimSpace(s))
	if e != nil || u.User != nil || u.Scheme != "https" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery {
		return ImportLink{}, false
	}
	service := ""
	switch u.Host {
	case "accoding.buaa.edu.cn":
	case "accoding.buaa.edu.cn:4000":
		service = "admin"
	default:
		return ImportLink{}, false
	}
	norm := func(s string) string {
		if len(s) > 64 {
			return ""
		}
		s = strings.TrimLeft(s, "0")
		return s
	}
	if m := problemLink.FindStringSubmatch(u.Path); m != nil && u.Fragment == "" {
		id := norm(m[1])
		return ImportLink{"problemset", id, service}, id != ""
	}
	var m []string
	if u.Path == "/contest-ng/index.html" {
		m = contestHash.FindStringSubmatch(u.Fragment)
	} else if u.Fragment == "" {
		m = contestLink.FindStringSubmatch(u.Path)
	}
	if m != nil {
		id := norm(m[1])
		return ImportLink{"contest", id, ""}, id != ""
	}
	return ImportLink{}, false
}

type ImportProblem struct {
	Target    accoding.Target   `json:"target"`
	Title     string            `json:"title"`
	Label     string            `json:"label"`
	SourceURL string            `json:"sourceUrl"`
	Samples   []accoding.Sample `json:"samples"`
	Warnings  []string          `json:"warnings"`
	Special   bool              `json:"special"`
 Statement string `json:"statement,omitempty"`
}
type ImportResult struct {
	AccountID string          `json:"accountId"`
	Title     string          `json:"title"`
	Problems  []ImportProblem `json:"problems"`
}

func projectProblem(p accoding.Problem) ImportProblem {
	t := p.Target
	u := accoding.StudentOrigin + "/contest-ng/index.html#/" + t.ContestID
	if t.Kind == "problemset" {
		origin := accoding.StudentOrigin
		if t.Service == "admin" {
			origin = accoding.AdminOrigin
		}
		u = origin + "/problem/" + t.ProblemID + "/index"
	}
	samples, warnings := p.Samples, p.Warnings
	if p.Special {
		samples = nil
		warnings = append(append([]string{}, warnings...), "特殊评测或交互题：不自动建立本地比较样例")
	}
	if samples == nil {
		samples = []accoding.Sample{}
	}
	if warnings == nil {
		warnings = []string{}
	}
	return ImportProblem{Target:t,Title:p.Title,Label:p.Label,SourceURL:u,Samples:samples,Warnings:warnings,Special:p.Special}
}
func (a *accountHandler) importRequest(ctx context.Context, method string, raw json.RawMessage) (json.RawMessage, *Failure) {
	var p struct {
		URL string `json:"url"`
	}
	if decode(raw, &p) != nil {
		return nil, &Failure{"invalid_params", "invalid link parameters"}
	}
	link, ok := parseLink(p.URL)
	if !ok {
		return nil, &Failure{"invalid_params", "unsupported Accoding link"}
	}
	if method == "resolve" {
		b, _ := json.Marshal(link)
		return b, nil
	}
	if a.user.ID == "" {
		return nil, &Failure{"auth", "login required"}
	}
	c, ok := a.client.(importClient)
	if !ok {
		return nil, &Failure{"method_not_found", "import unavailable"}
	}
	// Check current identity before reading any authenticated content.
	u, e := a.client.Account(ctx)
	if e != nil {
		if accoding.IsKind(e, accoding.Auth) {
			a.clear()
		}
		return nil, accountError(e)
	}
	if u.ID != a.user.ID {
		a.clear()
		return nil, &Failure{"auth", "identity changed"}
	}
	var documents []string
 result := ImportResult{AccountID: string(u.ID), Problems: []ImportProblem{}}
	if link.Kind == "contest" {
		v, e := c.ImportContest(ctx, link.ID)
		if e != nil {
			return nil, accountError(e)
		}
		result.Title = v.Title
		for _, p := range v.Problems {
			result.Problems = append(result.Problems, projectProblem(p))
   documents=append(documents,p.Statement.Content)
		}
	} else {
		v, e := c.GetProblem(ctx, accoding.Target{Kind: link.Kind, ProblemID: link.ID, Service: link.Service})
		if e != nil {
			return nil, accountError(e)
		}
		result.Title = v.Title
		result.Problems = append(result.Problems, projectProblem(v))
  documents=append(documents,v.Statement.Content)
	}
	if len(result.Problems) == 0 || len(result.Problems) > 200 {
		return nil, &Failure{"protocol", "unsupported problem count"}
	}
	if method=="import_document" {
  // The old import stays wire compatible. A separate opt-in method carries the local document.
  for i:=range result.Problems { if len(documents[i])>512<<10 {return nil,&Failure{"protocol","statement exceeds limit"}};result.Problems[i].Statement=documents[i] }
 }
 b, e := json.Marshal(result)
	if e != nil || len(b) > MaxLine-256 {
		return nil, &Failure{"protocol", "import exceeds message limit"}
	}
	return b, nil
}
