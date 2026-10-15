// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"testing"
)

// the account rules of the system backend, PAM replaced by a stub
func TestSystemRules(t *testing.T) {
	accounts := map[string]*account{
		"root":   {name: "root", uid: 0},
		"daemon": {name: "daemon", uid: 1, gid: 1},
		"alice":  {name: "alice", uid: 1000, gid: 1000},
		"bob":    {name: "bob", uid: 1001, gid: 20},
	}
	dialup := map[string]bool{"bob": true}
	newAuth := func(minUID int, root bool, groups []string) *systemAuth {
		return &systemAuth{
			service: "modemmenu", minUID: minUID, allowRoot: root, groups: groups,
			lookup: func(u string) (*account, error) {
				if a, ok := accounts[u]; ok {
					return a, nil
				}
				return nil, errNoUser
			},
			inGroups: func(a *account, g []string) (bool, error) { return dialup[a.name], nil },
			pam: func(service, user, password string) error {
				if password != "pw" {
					return errBadPassword
				}
				return nil
			},
		}
	}
	ctx := context.Background()
	for _, c := range []struct {
		a        *systemAuth
		user, pw string
		want     error
	}{
		{newAuth(1000, false, nil), "alice", "pw", nil},
		{newAuth(1000, false, nil), "alice", "px", errBadPassword},
		{newAuth(1000, false, nil), "nobody", "pw", errNoUser},
		{newAuth(1000, false, nil), "daemon", "pw", errNoUser},
		{newAuth(1000, false, nil), "root", "pw", errNoUser},
		{newAuth(1000, true, nil), "root", "pw", nil},
		{newAuth(1, false, nil), "daemon", "pw", nil},
		{newAuth(1000, false, []string{"dialup"}), "alice", "pw", errNoUser},
		{newAuth(1000, false, []string{"dialup"}), "bob", "pw", nil},
	} {
		err := c.a.authenticate(ctx, c.user, c.pw)
		if c.want == nil && err != nil || c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("%s/%s (min_uid %d, root %v, groups %v): %v, want %v",
				c.user, c.pw, c.a.minUID, c.a.allowRoot, c.a.groups, err, c.want)
		}
	}
	if !havePAM {
		if _, err := newSystemAuth(&section{name: "system", vals: map[string]string{}}); err == nil {
			t.Error("auth = system without PAM: no error")
		}
	}
}
