// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"fmt"
	"strings"
)

// systemAuth: the accounts of the system, by PAM: the auth and account
// stacks of pam_service, in a build with -tags pam.  Only accounts with a
// uid of at least min_uid (not root, unless allow_root), in one of groups
// if it is set, may log in.
type systemAuth struct {
	service   string
	minUID    int
	allowRoot bool
	groups    []string
	// the name service and PAM, replaced in tests
	lookup   func(string) (*account, error)
	inGroups func(*account, []string) (bool, error)
	pam      func(service, user, password string) error
}

// account: a user of the name service
type account struct {
	name     string
	uid, gid int
}

// newSystemAuth: the [system] backend (PAM and the name service);
// an error when this modemmenu is built without PAM
func newSystemAuth(s *section) (authenticator, error) {
	var err error
	a := &systemAuth{
		service:  s.get("pam_service", "modemmenu"),
		groups:   fields(s.get("groups", "")),
		lookup:   lookupNSS,
		inGroups: inGroupsNSS,
		pam:      pamAuthenticate,
	}
	if a.minUID, err = s.int("min_uid", 1000); err != nil {
		return nil, err
	}
	if a.allowRoot, err = s.bool("allow_root", false); err != nil {
		return nil, err
	}
	if !havePAM {
		return nil, fmt.Errorf("[general] auth = system: this modemmenu is built without PAM (make with libpam's headers, or go build -tags pam)")
	}
	return a, nil
}

// name: the backend's name, for {auth} and the log
func (a *systemAuth) name() string { return "system" }

// authenticate: the account looked up, min_uid, root and groups
// checked, then the password by PAM
func (a *systemAuth) authenticate(_ context.Context, user, password string) error {
	acct, err := a.lookup(user)
	if err != nil {
		return err
	}
	if err := a.allowed(acct); err != nil {
		return err
	}
	return a.pam(a.service, user, password)
}

// allowed: an account that may dial in.  The others are not this
// backend's users: the next one is asked
func (a *systemAuth) allowed(acct *account) error {
	if acct.uid == 0 && !a.allowRoot {
		return fmt.Errorf("%w: %s is root", errNoUser, acct.name)
	}
	if acct.uid != 0 && acct.uid < a.minUID {
		return fmt.Errorf("%w: %s has uid %d < min_uid %d", errNoUser, acct.name, acct.uid, a.minUID)
	}
	if len(a.groups) == 0 {
		return nil
	}
	in, err := a.inGroups(acct, a.groups)
	if err != nil {
		return err
	}
	if !in {
		return fmt.Errorf("%w: %s is in none of %s", errNoUser, acct.name, strings.Join(a.groups, ", "))
	}
	return nil
}
