// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/D-Modem/menu/internal/config"
)

// the account rules of the system backend, PAM replaced by a stub
func TestSystemRules(t *testing.T) {
	accounts := map[string]*Account{
		"root":   {name: "root", uid: 0},
		"daemon": {name: "daemon", uid: 1, gid: 1},
		"alice":  {name: "alice", uid: 1000, gid: 1000},
		"bob":    {name: "bob", uid: 1001, gid: 20},
	}
	dialup := map[string]bool{"bob": true}
	newAuth := func(minUID int, root bool, groups []string) *System {
		return &System{
			service: "modemmenu", minUID: minUID, allowRoot: root, groups: groups,
			lookup: func(u string) (*Account, error) {
				if a, ok := accounts[u]; ok {
					return a, nil
				}
				return nil, ErrNoUser
			},
			inGroups: func(a *Account, g []string) (bool, error) { return dialup[a.name], nil },
			pam: func(service, user, password string) error {
				if password != "pw" {
					return ErrBadPassword
				}
				return nil
			},
		}
	}
	ctx := context.Background()
	for _, c := range []struct {
		a        *System
		user, pw string
		want     error
	}{
		{newAuth(1000, false, nil), "alice", "pw", nil},
		{newAuth(1000, false, nil), "alice", "px", ErrBadPassword},
		{newAuth(1000, false, nil), "nobody", "pw", ErrNoUser},
		{newAuth(1000, false, nil), "daemon", "pw", ErrNoUser},
		{newAuth(1000, false, nil), "root", "pw", ErrNoUser},
		{newAuth(1000, true, nil), "root", "pw", nil},
		{newAuth(1, false, nil), "daemon", "pw", nil},
		{newAuth(1000, false, []string{"dialup"}), "alice", "pw", ErrNoUser},
		{newAuth(1000, false, []string{"dialup"}), "bob", "pw", nil},
	} {
		err := c.a.Authenticate(ctx, c.user, c.pw)
		if c.want == nil && err != nil || c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("%s/%s (min_uid %d, root %v, groups %v): %v, want %v",
				c.user, c.pw, c.a.minUID, c.a.allowRoot, c.a.groups, err, c.want)
		}
	}
	if !havePAM {
		if _, err := NewSystem(&config.Section{Name: "system", Vals: map[string]string{}}); err == nil {
			t.Error("auth = system without PAM: no error")
		}
	}
}
