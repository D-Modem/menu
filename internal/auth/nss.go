// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"errors"
	"os/user"
	"strconv"
)

// LookupNSS: the account from the name service (getpwnam with cgo)
func LookupNSS(name string) (*Account, error) {
	u, err := user.Lookup(name)
	var unknown user.UnknownUserError
	if errors.As(err, &unknown) {
		return nil, ErrNoUser
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
	return &Account{name: u.Username, uid: uid, gid: gid}, nil
}

// InGroupsNSS: whether the account is in any of the groups, by the
// name service (/etc/group, LDAP via NSS...)
func InGroupsNSS(acct *Account, groups []string) (bool, error) {
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
