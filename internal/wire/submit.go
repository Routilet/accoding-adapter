package wire

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/Routilet/accoding-adapter/accoding"
	"strings"
	"time"
)

type submitClient interface {
	Prepare(context.Context, accoding.Target) (accoding.Prepared, error)
	Submit(context.Context, accoding.Target, accoding.ID, string, string, accoding.Journal) (accoding.Submission, error)
}
type submitPlan struct {
	Token      string          `json:"token"`
	Target     accoding.Target `json:"target"`
	AccountID  string          `json:"accountId"`
	Languages  []string        `json:"languages"`
	SourceHash string          `json:"sourceHash"`
	expires    time.Time
}
type submitReceipt struct {
	ID         string          `json:"id"`
	Target     accoding.Target `json:"target"`
	AccountID  string          `json:"accountId"`
	Language   string          `json:"language"`
	Result     string          `json:"result"`
	SourceHash string          `json:"sourceHash"`
}

func validHash(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && strings.ToLower(s) == s
}
func (a *accountHandler) submissionRequest(ctx context.Context, method string, raw json.RawMessage) (json.RawMessage, *Failure) {
	invalid := &Failure{"invalid_params", "invalid submission parameters"}
	// A new prepare or any submit consumes the previous token, even on error.
	previous := a.prepared
	a.prepared = nil
	if method == "prepare" {
		var p struct {
			Target     accoding.Target `json:"target"`
			SourceHash string          `json:"sourceHash"`
		}
		if decode(raw, &p) != nil || !validHash(p.SourceHash) {
			return nil, invalid
		}
		if a.user.ID == "" {
			return nil, &Failure{"auth", "login required"}
		}
		c, ok := a.client.(submitClient)
		if !ok || a.journal == nil {
			return nil, &Failure{"submission_unavailable", "submission storage unavailable"}
		}
		v, e := c.Prepare(ctx, p.Target)
		if e != nil {
			return nil, accountError(e)
		}
		if v.Account != a.user.ID || v.Target != p.Target || len(v.Languages) == 0 || len(v.Languages) > 128 {
			return nil, &Failure{"protocol", "invalid submission target"}
		}
		var token [16]byte
		if _, e = rand.Read(token[:]); e != nil {
			return nil, &Failure{"protocol", "cannot create confirmation"}
		}
		plan := submitPlan{hex.EncodeToString(token[:]), v.Target, string(v.Account), v.Languages, p.SourceHash, time.Now().Add(5 * time.Minute)}
		a.prepared = &plan
		b, _ := json.Marshal(plan)
		return b, nil
	}
	var p struct {
		Token    string `json:"token"`
		Language string `json:"language"`
		Code     string `json:"code"`
	}
	if decode(raw, &p) != nil || p.Token == "" || p.Language == "" || len(p.Language) > 128 || len(p.Code) == 0 || len(p.Code) > 4<<20 || strings.ContainsRune(p.Code, 0) {
		return nil, invalid
	}
	if a.user.ID == "" {
		return nil, &Failure{"auth", "login required"}
	}
	if previous == nil || previous.Token != p.Token || previous.AccountID != string(a.user.ID) || time.Now().After(previous.expires) {
		return nil, &Failure{"confirmation_expired", "prepare again"}
	}
	h := sha256.Sum256([]byte(p.Code))
	if hex.EncodeToString(h[:]) != previous.SourceHash {
		return nil, &Failure{"confirmation_expired", "source changed"}
	}
	allowed := false
	for _, l := range previous.Languages {
		if l == p.Language {
			allowed = true
		}
	}
	if !allowed {
		return nil, invalid
	}
	c, ok := a.client.(submitClient)
	if !ok || a.journal == nil {
		return nil, &Failure{"submission_unavailable", "submission storage unavailable"}
	}
	v, e := c.Submit(ctx, previous.Target, a.user.ID, p.Language, p.Code, a.journal)
	if e != nil {
		if accoding.IsKind(e, accoding.Unknown) {
			return nil, &Failure{"unknown_outcome", "check OJ before any further submission"}
		}
		return nil, accountError(e)
	}
	b, _ := json.Marshal(submitReceipt{string(v.ID), previous.Target, previous.AccountID, p.Language, v.Result, previous.SourceHash})
	return b, nil
}

type pendingJournal interface {
	accoding.Journal
	Pending(context.Context, accoding.ID, string) (*accoding.PendingAttempt, error)
}
type recoveryClient interface {
	RecoverAttempt(context.Context, accoding.Attempt) (*accoding.Submission, error)
}
type reconciliation struct {
	State  string `json:"state"`
	ID     string `json:"id,omitempty"`
	Result string `json:"result,omitempty"`
}

// Reconcile is read-only at OJ. It may mark a positively identified local attempt resolved.
func (a *accountHandler) reconcile(ctx context.Context, raw json.RawMessage) (json.RawMessage, *Failure) {
	var p struct {
		ProblemID string `json:"problemId"`
	}
	if decode(raw, &p) != nil || p.ProblemID == "" || len(p.ProblemID) > 64 {
		return nil, &Failure{"invalid_params", "invalid problem id"}
	}
	if a.user.ID == "" {
		return nil, &Failure{"auth", "login required"}
	}
	j, ok := a.journal.(pendingJournal)
	c, ok2 := a.client.(recoveryClient)
	if !ok || !ok2 {
		return nil, &Failure{"submission_unavailable", "submission storage unavailable"}
	}
	u, e := a.client.Account(ctx)
	if e != nil {
		return nil, accountError(e)
	}
	if u.ID != a.user.ID {
		a.clear()
		return nil, &Failure{"auth", "identity changed"}
	}
	pending, e := j.Pending(ctx, u.ID, p.ProblemID)
	if e != nil {
		return nil, &Failure{"protocol", "cannot inspect pending submission"}
	}
	reply := func(state, id, result string) (json.RawMessage, *Failure) {
		b, _ := json.Marshal(reconciliation{state, id, result})
		return b, nil
	}
	if pending == nil {
		return reply("none", "", "")
	}
	if pending.Attempt.Target.Kind != "problemset" {
		return reply("manual_required", "", "")
	}
	s, e := c.RecoverAttempt(ctx, pending.Attempt)
	if e != nil {
		return nil, accountError(e)
	}
	if s == nil {
		return reply("unresolved", "", "")
	}
	if e = j.Resolve(ctx, pending.Key, *s); e != nil {
		return nil, &Failure{"unknown_outcome", "cannot persist recovered submission"}
	}
	return reply("resolved", string(s.ID), s.Result)
}
