package wire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/Routilet/accoding-adapter/accoding"
	"testing"
)

type fakeSubmit struct {
	fakeAccount
	sends int
	err   error
}

func (f *fakeSubmit) Prepare(_ context.Context, t accoding.Target) (accoding.Prepared, error) {
	return accoding.Prepared{Target: t, Account: f.user.ID, Languages: []string{"C99"}}, nil
}
func (f *fakeSubmit) Submit(_ context.Context, t accoding.Target, a accoding.ID, l, c string, j accoding.Journal) (accoding.Submission, error) {
	f.sends++
	return accoding.Submission{ID: "123", Target: t, CreatorID: a, ProblemID: accoding.ID(t.ProblemID), Lang: l, Result: "WT"}, f.err
}
func TestSubmitConfirmation(t *testing.T) {
	ctx := context.Background()
	f := &fakeSubmit{}
	a := accountHandler{client: f, journal: accoding.FileJournal{Directory: t.TempDir()}}
	code := "int main(void){return 0;}"
	h := sha256.Sum256([]byte(code))
	target := accoding.Target{Kind: "contest", ContestID: "42", ProblemID: "5"}
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	prep := raw(map[string]any{"target": target, "sourceHash": hex.EncodeToString(h[:])})
	if _, e := a.submissionRequest(ctx, "prepare", prep); e == nil || e.Code != "auth" {
		t.Fatal(e)
	}
	a.handle(ctx, "login", raw(map[string]string{"username": "7", "password": "correct"}))
	plan := func() submitPlan {
		b, e := a.submissionRequest(ctx, "prepare", prep)
		if e != nil {
			t.Fatal(e)
		}
		var p submitPlan
		if json.Unmarshal(b, &p) != nil || p.AccountID != "7" || p.Token == "" {
			t.Fatal(string(b))
		}
		return p
	}
	send := func(p submitPlan, code, lang string) (json.RawMessage, *Failure) {
		return a.submissionRequest(ctx, "submit", raw(map[string]string{"token": p.Token, "code": code, "language": lang}))
	}
	p := plan()
	if _, e := send(p, code+" ", "C99"); e == nil || e.Code != "confirmation_expired" {
		t.Fatal(e)
	}
	if _, e := send(p, code, "C99"); e == nil {
		t.Fatal("replayed")
	}
	p = plan()
	if _, e := send(p, code, "C++"); e == nil {
		t.Fatal("wrong language")
	}
	p = plan()
	a.handle(ctx, "login", raw(map[string]string{"username": "8", "password": "correct"}))
	if _, e := send(p, code, "C99"); e == nil {
		t.Fatal("switched account")
	}
	if f.sends != 0 {
		t.Fatal(f.sends)
	}
	// New account must explicitly prepare and confirm.
	b, e := a.submissionRequest(ctx, "prepare", prep)
	if e != nil {
		t.Fatal(e)
	}
	json.Unmarshal(b, &p)
	b, e = send(p, code, "C99")
	if e != nil {
		t.Fatal(e)
	}
	var r submitReceipt
	if json.Unmarshal(b, &r) != nil || r.AccountID != "8" || r.ID != "123" || r.SourceHash != hex.EncodeToString(h[:]) {
		t.Fatal(string(b))
	}
	if _, e = send(p, code, "C99"); e == nil || f.sends != 1 {
		t.Fatal(e, f.sends)
	}
	b, e = a.submissionRequest(ctx, "prepare", prep)
	if e != nil {
		t.Fatal(e)
	}
	json.Unmarshal(b, &p)
	f.err = &accoding.Error{Kind: accoding.Unknown, Message: "PRIVATE"}
	if _, e = send(p, code, "C99"); e == nil || e.Code != "unknown_outcome" {
		t.Fatal(e)
	}
}
