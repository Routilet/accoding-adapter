package wire

import (
	"context"
	"encoding/json"
	"github.com/Routilet/accoding-adapter/accoding"
	"strings"
	"testing"
)

type fakeImport struct {
	fakeAccount
	calls int
}

func (f *fakeImport) ImportContest(_ context.Context, id string) (accoding.Contest, error) {
	f.calls++
	return accoding.Contest{Title: "比赛", Problems: []accoding.Problem{{Title: "题", Label: "A", Target: accoding.Target{Kind: "contest", ContestID: id, ProblemID: "8"}, Statement: accoding.Statement{Content: "DO NOT EXPORT"}, Samples: []accoding.Sample{{Input: "1\n", Expected: "2\n"}}}}}, nil
}
func (f *fakeImport) GetProblem(_ context.Context, t accoding.Target) (accoding.Problem, error) {
	f.calls++
	return accoding.Problem{Target: t, Title: "题", Label: "#" + t.ProblemID, Special: true, Samples: []accoding.Sample{{Input: "1", Expected: "2"}}}, nil
}
func TestLinks(t *testing.T) {
	for _, s := range []string{"https://accoding.buaa.edu.cn/problem/42/index", "https://accoding.buaa.edu.cn:4000/problem/42", "https://accoding.buaa.edu.cn/contest-ng/index.html#/42/problem/A", "https://accoding.buaa.edu.cn:4000/contest/42/edit"} {
		if l, ok := parseLink(s); !ok || l.ID != "42" {
			t.Fatal(s, l)
		}
	}
	for _, s := range []string{"http://accoding.buaa.edu.cn/problem/42", "https://accoding.buaa.edu.cn.evil.test/problem/42", "https://x@accoding.buaa.edu.cn/problem/42", "https://accoding.buaa.edu.cn/problem/../42", "https://accoding.buaa.edu.cn/problem/42?x=y", "https://accoding.buaa.edu.cn/problem/0"} {
		if _, ok := parseLink(s); ok {
			t.Fatal(s)
		}
	}
}
func TestImport(t *testing.T) {
	f := &fakeImport{}
	a := accountHandler{client: f}
	ctx := context.Background()
	raw := json.RawMessage(`{"url":"https://accoding.buaa.edu.cn/contest-ng/index.html#/42"}`)
	if _, e := a.importRequest(ctx, "resolve", raw); e != nil || f.calls != 0 || f.checks != 0 {
		t.Fatal(e)
	}
	if _, e := a.importRequest(ctx, "import", raw); e == nil || e.Code != "auth" {
		t.Fatal(e)
	}
	a.handle(ctx, "login", json.RawMessage(`{"username":"42","password":"correct"}`))
	b, e := a.importRequest(ctx, "import", raw)
	if e != nil || f.calls != 1 || strings.Contains(string(b), "DO NOT EXPORT") {
		t.Fatal(e, string(b))
	}
	var r ImportResult
	json.Unmarshal(b, &r)
	if r.AccountID != "42" || r.Problems[0].Target.ProblemID != "8" || r.Problems[0].Samples[0].Expected != "2\n" {
		t.Fatal(r)
	}
	b, e = a.importRequest(ctx, "import", json.RawMessage(`{"url":"https://accoding.buaa.edu.cn:4000/problem/8/index"}`))
	if e != nil {
		t.Fatal(e)
	}
	json.Unmarshal(b, &r)
	if len(r.Problems[0].Samples) != 0 || !r.Problems[0].Special || r.Problems[0].Target.Service != "admin" {
		t.Fatal(r)
	}
}
