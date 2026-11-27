package wire

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/Routilet/accoding-adapter/accoding"
	"strings"
	"testing"
)

type fakeAccount struct {
	user                    accoding.User
	logins, checks, logouts int
	checkErr                error
}

func (f *fakeAccount) Login(_ context.Context, u, p string) (accoding.User, error) {
	f.logins++
	if p != "correct" {
		return accoding.User{}, &accoding.Error{Kind: accoding.Auth, Message: "SECRET"}
	}
	f.user = accoding.User{ID: accoding.ID(u), Nickname: "学生"}
	return f.user, nil
}
func (f *fakeAccount) Account(context.Context) (accoding.User, error) {
	f.checks++
	return f.user, f.checkErr
}
func (f *fakeAccount) Logout() { f.logouts++; f.user = accoding.User{} }
func TestAccountLifecycle(t *testing.T) {
	f := &fakeAccount{}
	var out bytes.Buffer
	in := `{"id":1,"method":"account","params":{}}
{"id":2,"method":"login","params":{"username":"42","password":"correct"}}
{"id":3,"method":"account","params":{}}
{"id":4,"method":"account","params":{"refresh":true}}
{"id":5,"method":"login","params":{"username":"43","password":"wrong"}}
{"id":6,"method":"account","params":{}}
{"id":7,"method":"logout","params":{}}
`
	if err := serve(strings.NewReader(in), &out, f); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "SECRET") || strings.Contains(out.String(), "correct") {
		t.Fatal("secret leaked")
	}
	d := json.NewDecoder(&out)
	for i, w := range []string{"signed_out", "signed_in", "signed_in", "signed_in", "auth", "signed_out", "signed_out"} {
		var r Response
		if err := d.Decode(&r); err != nil {
			t.Fatal(err)
		}
		if w == "auth" {
			if r.Error == nil || r.Error.Code != w {
				t.Fatal(r)
			}
			continue
		}
		var s AccountState
		if json.Unmarshal(r.Result, &s) != nil || s.State != w || s.State == "signed_in" && s.ID != "42" {
			t.Fatalf("%d %+v", i, s)
		}
	}
	if f.logins != 2 || f.checks != 1 {
		t.Fatal(f)
	}
}
func TestAccountExpiryAndNetwork(t *testing.T) {
	f := &fakeAccount{}
	a := accountHandler{client: f}
	ctx := context.Background()
	a.handle(ctx, "login", json.RawMessage(`{"username":"1","password":"correct"}`))
	f.checkErr = &accoding.Error{Kind: accoding.Network}
	_, e := a.handle(ctx, "account", json.RawMessage(`{"refresh":true}`))
	if e.Code != "network" || a.state().State != "signed_in" {
		t.Fatal(e)
	}
	f.checkErr = &accoding.Error{Kind: accoding.Auth}
	_, e = a.handle(ctx, "account", json.RawMessage(`{"refresh":true}`))
	if e.Code != "auth" || a.state().State != "signed_out" {
		t.Fatal(e)
	}
}
