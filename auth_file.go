// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

// fileAuth: a text file of user:password lines (the password hashed as
// checkPassword reads it, ! before it for a locked account; # comments),
// read at each login so that edits take effect at once
type fileAuth struct {
	path   string
	scheme string
}

// newFileAuth: the [file] backend, its users file checked to exist
func newFileAuth(s *section) (authenticator, error) {
	a := &fileAuth{
		path:   s.get("path", "/etc/asterisk/modemmenu.users"),
		scheme: strings.ToLower(s.get("hash", "auto")),
	}
	if _, err := os.Stat(a.path); err != nil {
		return nil, fmt.Errorf("[file] path: %v", err)
	}
	return a, nil
}

// name: the backend's name, for {auth} and the log
func (a *fileAuth) name() string { return "file" }

// authenticate: the user's line in the users file and its hash
// checked; a locked entry (!) is refused
func (a *fileAuth) authenticate(_ context.Context, user, password string) error {
	f, err := os.Open(a.path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		name, stored, ok := strings.Cut(line, ":")
		if !ok || name != user {
			continue
		}
		// a locked account (signup's pending ones): !hash
		if strings.HasPrefix(stored, "!") {
			burn(password)
			return fmt.Errorf("%w: %s is locked", errBadPassword, user)
		}
		// user:hash[:anything else, e.g. a full name]
		if a.scheme != "plain" && !strings.HasPrefix(stored, "{") {
			stored, _, _ = strings.Cut(stored, ":")
		}
		ok, err := checkPassword(stored, password, a.scheme)
		if err != nil {
			return fmt.Errorf("%s: user %s: %v", a.path, user, err)
		}
		if !ok {
			return errBadPassword
		}
		return nil
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return errNoUser
}
