package wire

import (
	"context"
	"encoding/json"
	"github.com/Routilet/accoding-adapter/accoding"
	"testing"
)

type fakeJudge struct {
	fakeAccount
	rows []accoding.Submission
}

func (f *fakeJudge) ListSubmissions(context.Context, accoding.Target) ([]accoding.Submission, error) {
	return f.rows, nil
}
func (f *fakeJudge) GetSubmission(context.Context, accoding.Target, string) (accoding.Submission, error) {
	return f.rows[0], nil
}
func TestJudgeStatus(t *testing.T) {
	ctx := context.Background()
	f := &fakeJudge{}
	a := accountHandler{client: f}
	a.handle(ctx, "login", json.RawMessage(`{"username":"7","password":"correct"}`))
	target := accoding.Target{Kind: "contest", ContestID: "42", ProblemID: "5"}
	f.rows = []accoding.Submission{{ID: "99", CreatorID: "7", ProblemID: "5", Target: target, Result: "AC", Lang: "C99"}, {ID: "100", CreatorID: "7", ProblemID: "5", Target: target, Result: "WA", Lang: "C99"}}
	b, _ := json.Marshal(map[string]any{"target": target})
	raw, e := a.judge(ctx, "status", b)
	var s judgeStatus
	if e != nil || json.Unmarshal(raw, &s) != nil || !s.Accepted || s.Latest.ID != "100" || s.Latest.Result != "WA" {
		t.Fatal(string(raw), e)
	}
	b, _ = json.Marshal(map[string]any{"target": target, "id": "99"})
	if _, e = a.judge(ctx, "result", b); e != nil {
		t.Fatal(e)
	}
	f.rows[0].CreatorID = "8"
	if _, e = a.judge(ctx, "result", b); e == nil || e.Code != "protocol" {
		t.Fatal(e)
	}
	a.clear()
	if _, e = a.judge(ctx, "result", b); e == nil || e.Code != "auth" {
		t.Fatal(e)
	}
}
