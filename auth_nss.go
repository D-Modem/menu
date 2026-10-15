// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"os/user"
	"strconv"
)

// lookupNSS: the account from the name service (getpwnam with cgo)
func lookupNSS(name string) (*account, error) {
	u, err := user.Lookup(name)
	var unknown user.UnknownUserError
	if errors.As(err, &unknown) {
		return nil, errNoUser
	}
	if err != nil {
		return nil, err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return nil, err
	}
	return &account{name: u.Username, uid: uid, gid: gid}, nil
}

func inGroupsNSS(acct *account, groups []string) (bool, error) {
	u, err := user.Lookup(acct.name)
	if err != nil {
		return false, err
	}
	ids, err := u.GroupIds()
	if err != nil {
		return false, err
	}
	have := map[string]bool{}
	for _, id := range ids {
		have[id] = true
	}
	for _, name := range groups {
		if g, err := user.LookupGroup(name); err == nil && have[g.Gid] {
			return true, nil
		}
	}
	return false, nil
}
