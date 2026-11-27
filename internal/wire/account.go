package wire

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Routilet/accoding-adapter/accoding"
)

type accountClient interface {
	Login(context.Context, string, string) (accoding.User, error)
	Account(context.Context) (accoding.User, error)
	Logout()
}
type AccountState struct {
	State    string `json:"state"`
	ID       string `json:"id,omitempty"`
	Nickname string `json:"nickname,omitempty"`
}
type accountHandler struct {
	client accountClient
	user   accoding.User
}

func (a *accountHandler) clear() { a.client.Logout(); a.user = accoding.User{} }
func (a *accountHandler) state() AccountState {
	if a.user.ID == "" {
		return AccountState{State: "signed_out"}
	}
	return AccountState{State: "signed_in", ID: string(a.user.ID), Nickname: a.user.Nickname}
}
func accountError(err error) *Failure {
	var e *accoding.Error
	if errors.As(err, &e) {
		switch e.Kind {
		case accoding.Auth:
			return &Failure{"auth", "authentication failed or expired"}
		case accoding.Forbidden:
			return &Failure{"forbidden", "access denied"}
		case accoding.Network:
			return &Failure{"network", "network unavailable"}
		case accoding.Rate:
			return &Failure{"rate", "rate limited"}
		case accoding.Server:
			return &Failure{"server", "service unavailable"}
		case accoding.Cancelled:
			return &Failure{"cancelled", "operation cancelled"}
		}
	}
	return &Failure{"protocol", "account operation failed"}
}
func (a *accountHandler) handle(ctx context.Context, method string, raw json.RawMessage) (json.RawMessage, *Failure) {
	invalid := &Failure{"invalid_params", "invalid account parameters"}
	switch method {
	case "login":
		var p struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if decode(raw, &p) != nil || p.Username == "" || p.Password == "" || len(p.Username) > 1024 || len(p.Password) > 4096 {
			return nil, invalid
		}
		// Switching accounts is explicit; a failed switch never retains the prior identity.
		a.clear()
		u, err := a.client.Login(ctx, p.Username, p.Password)
		if err != nil {
			a.clear()
			return nil, accountError(err)
		}
		if u.ID == "" {
			a.clear()
			return nil, &Failure{"protocol", "invalid identity"}
		}
		a.user = u
	case "account":
		var p struct {
			Refresh bool `json:"refresh"`
		}
		if decode(raw, &p) != nil {
			return nil, invalid
		}
		if p.Refresh && a.user.ID != "" {
			u, err := a.client.Account(ctx)
			if err != nil {
				if accoding.IsKind(err, accoding.Auth) || accoding.IsKind(err, accoding.Forbidden) {
					a.clear()
				}
				return nil, accountError(err)
			}
			if u.ID != a.user.ID {
				a.clear()
				return nil, &Failure{"auth", "identity changed"}
			}
			a.user = u
		}
	case "logout":
		if decode(raw, &struct{}{}) != nil {
			return nil, invalid
		}
		a.clear()
	}
	b, _ := json.Marshal(a.state())
	return b, nil
}
