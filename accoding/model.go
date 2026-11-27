package accoding

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

type ID string

func (i *ID) UnmarshalJSON(b []byte) error {
	var s string
	if len(b) > 0 && b[0] == '"' {
		if json.Unmarshal(b, &s) != nil {
			return fail(Protocol, "ID 无效")
		}
	} else {
		s = string(b)
	}
	if !validID(s) {
		return fail(Protocol, "ID 无效")
	}
	*i = ID(s)
	return nil
}

type User struct {
	ID       ID     `json:"id"`
	Nickname string `json:"nickname"`
}
type Target struct {
	Kind      string `json:"kind"`
	ContestID string `json:"contestId,omitempty"`
	ProblemID string `json:"problemId"`
	Service   string `json:"service,omitempty"`
}

func (t Target) validate() error {
	if !validID(t.ProblemID) {
		return fail(Protocol, "题目 ID 无效")
	}
	if t.Kind == "contest" && validID(t.ContestID) && t.Service == "" || t.Kind == "problemset" && t.ContestID == "" && (t.Service == "" || t.Service == "admin") {
		return nil
	}
	return fail(Protocol, "题目来源无效")
}

type Sample struct {
	Input    string `json:"input"`
	Expected string `json:"expected"`
}
type Statement struct {
	Format  string `json:"format"`
	Content string `json:"content"`
	BaseURL string `json:"baseUrl"`
}
type Problem struct {
	Target      Target    `json:"target"`
	Title       string    `json:"title"`
	Label       string    `json:"label"`
	Statement   Statement `json:"statement"`
	Languages   []string  `json:"languages"`
	Samples     []Sample  `json:"samples"`
	Warnings    []string  `json:"warnings"`
	TimeLimit   string    `json:"timeLimit,omitempty"`
	MemoryLimit string    `json:"memoryLimit,omitempty"`
	Special     bool      `json:"special"`
	order       int
}
type Contest struct {
	ID        ID        `json:"id"`
	Title     string    `json:"title"`
	StartTime string    `json:"startTime"`
	EndTime   string    `json:"endTime"`
	Problems  []Problem `json:"problems"`
}

func (c Contest) Active(now time.Time) bool {
	if t, e := time.Parse(time.RFC3339, c.StartTime); e == nil && now.Before(t) {
		return false
	}
	if t, e := time.Parse(time.RFC3339, c.EndTime); e == nil && !now.Before(t) {
		return false
	}
	return true
}

type Submission struct {
	ID        ID              `json:"id"`
	Target    Target          `json:"target"`
	Result    string          `json:"result"`
	ProblemID ID              `json:"problem_id"`
	CreatorID ID              `json:"creator_id"`
	Lang      string          `json:"lang"`
	Language  string          `json:"language"`
	Score     json.RawMessage `json:"score"`
	Detail    string          `json:"detail"`
	CreatedAt string          `json:"created_at"`
}

func (s Submission) Pending() bool { return s.Result == "WT" || s.Result == "JG" }
func (s *Submission) verify(t Target, account ID) error {
	if !validID(string(s.ID)) || s.Result == "" || s.ProblemID != "" && string(s.ProblemID) != t.ProblemID || s.CreatorID != "" && s.CreatorID != account {
		return fail(Protocol, "提交记录与账号或题目不匹配")
	}
	s.Target = t
	if s.Lang == "" {
		s.Lang = s.Language
	}
	if len(s.Detail) > 65536 {
		s.Detail = s.Detail[:65536]
	}
	return nil
}
func scalar(r json.RawMessage) string {
	var s string
	if json.Unmarshal(r, &s) == nil {
		return s
	}
	if _, e := strconv.ParseFloat(string(r), 64); e == nil {
		return string(r)
	}
	return ""
}
func languages(s string) []string {
	var a []string
	for _, l := range strings.Split(s, ",") {
		if l = strings.TrimSpace(l); l != "" {
			a = append(a, l)
		}
	}
	return a
}
func includes(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
