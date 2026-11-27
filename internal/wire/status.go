package wire

import (
	"context"
	"encoding/json"
	"github.com/Routilet/accoding-adapter/accoding"
	"regexp"
)

type statusClient interface {
	ListSubmissions(context.Context, accoding.Target) ([]accoding.Submission, error)
	GetSubmission(context.Context, accoding.Target, string) (accoding.Submission, error)
}
type judgeResult struct {
	ID       string `json:"id"`
	Result   string `json:"result"`
	Language string `json:"language"`
 Score string `json:"score,omitempty"`
 Detail string `json:"detail,omitempty"`
 CreatedAt string `json:"createdAt,omitempty"`
}
type judgeStatus struct {
	AccountID string          `json:"accountId"`
	Target    accoding.Target `json:"target"`
	Accepted  bool            `json:"accepted"`
	Latest    *judgeResult    `json:"latest,omitempty"`
}

var judgeID = regexp.MustCompile(`^[1-9][0-9]{0,63}$`)

func laterID(a, b string) bool { return len(a) > len(b) || len(a) == len(b) && a > b }
func checkedJudge(s accoding.Submission, t accoding.Target, u accoding.ID) (judgeResult, bool) {
	if !judgeID.MatchString(string(s.ID)) || s.CreatorID != u || string(s.ProblemID) != t.ProblemID || s.Target != t || s.Result == "" || len(s.Result) > 128 || len(s.Lang) > 128 {
		return judgeResult{}, false
	}
	return judgeResult{ID:string(s.ID),Result:s.Result,Language:s.Lang}, true
}
func (a *accountHandler) judge(ctx context.Context, method string, raw json.RawMessage) (json.RawMessage, *Failure) {
	var p struct {
		Target accoding.Target `json:"target"`
		ID     string          `json:"id,omitempty"`
	}
	if decode(raw, &p) != nil || (method == "result" || method=="result_detail") && !judgeID.MatchString(p.ID) || (method == "status" || method=="status_detail") && p.ID != "" {
		return nil, &Failure{"invalid_params", "invalid judge query"}
	}
	if a.user.ID == "" {
		return nil, &Failure{"auth", "login required"}
	}
	c, ok := a.client.(statusClient)
	if !ok {
		return nil, &Failure{"method_not_found", "judge query unavailable"}
	}
	u, e := a.client.Account(ctx)
	if e != nil {
		return nil, accountError(e)
	}
	if u.ID != a.user.ID {
		a.clear()
		return nil, &Failure{"auth", "identity changed"}
	}
	out := judgeStatus{AccountID: string(u.ID), Target: p.Target}
	var records []accoding.Submission
	if method == "result" || method=="result_detail" {
		var s accoding.Submission
		s, e = c.GetSubmission(ctx, p.Target, p.ID)
		records = []accoding.Submission{s}
	} else {
		records, e = c.ListSubmissions(ctx, p.Target)
	}
	if e != nil {
		return nil, accountError(e)
	}
	for _, s := range records {
		r, valid := checkedJudge(s, p.Target, u.ID)
		if !valid || (method == "result" || method=="result_detail") && r.ID != p.ID {
			return nil, &Failure{"protocol", "judge identity mismatch"}
		}
		if method=="status_detail"||method=="result_detail"{r.Score=string(s.Score);r.Detail=s.Detail;r.CreatedAt=s.CreatedAt}
  if r.Result == "AC" {
			out.Accepted = true
		}
		if out.Latest == nil || laterID(r.ID, out.Latest.ID) {
			copy := r
			out.Latest = &copy
		}
	}
	b, _ := json.Marshal(out)
	return b, nil
}
