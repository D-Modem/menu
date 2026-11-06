// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/D-Modem/menu/internal/config"
)

// System: the accounts of the system, by PAM: the auth and account
// stacks of pam_service, in a build with -tags pam.  Only accounts with a
// uid of at least min_uid (not root, unless allow_root), in one of groups
// if it is set, may log in.
type System struct {
	service   string
	minUID    int
	allowRoot bool
	groups    []string
	// the name service and PAM, replaced in tests
	lookup   func(string) (*Account, error)
	inGroups func(*Account, []string) (bool, error)
	pam      func(service, user, password string) error
}

// Account: a user of the name service
type Account struct {
	name     string
	uid, gid int
}

// NewSystem: the [system] backend (PAM and the name service);
// an error when this modemmenu is built without PAM
func NewSystem(s *config.Section) (Authenticator, error) {
	var err error
	a := &System{
		service:  s.Get("pam_service", "modemmenu"),
		groups:   config.Fields(s.Get("groups", "")),
		lookup:   LookupNSS,
		inGroups: InGroupsNSS,
		pam:      pamAuthenticate,
	}
	if a.minUID, err = s.Int("min_uid", 1000); err != nil {
		return nil, err
	}
	if a.allowRoot, err = s.Bool("allow_root", false); err != nil {
		return nil, err
	}
	if !havePAM {
		return nil, fmt.Errorf("[general] auth = system: this modemmenu is built without PAM (make with libpam's headers, or go build -tags pam)")
	}
	return a, nil
}

// Name: the backend's Name, for {auth} and the log
func (a *System) Name() string { return "system" }

// Authenticate: the account looked up, min_uid, root and groups
// checked, then the password by PAM
func (a *System) Authenticate(_ context.Context, user, password string) error {
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
func (a *System) allowed(acct *Account) error {
	if acct.uid == 0 && !a.allowRoot {
		return fmt.Errorf("%w: %s is root", ErrNoUser, acct.name)
	}
	if acct.uid != 0 && acct.uid < a.minUID {
		return fmt.Errorf("%w: %s has uid %d < min_uid %d", ErrNoUser, acct.name, acct.uid, a.minUID)
	}
	if len(a.groups) == 0 {
		return nil
	}
	in, err := a.inGroups(acct, a.groups)
	if err != nil {
		return err
	}
	if !in {
		return fmt.Errorf("%w: %s is in none of %s", ErrNoUser, acct.name, strings.Join(a.groups, ", "))
	}
	return nil
}
