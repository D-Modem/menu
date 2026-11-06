// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/D-Modem/menu/internal/config"
)

// File: a text file of user:password lines (the password hashed as
// checkPassword reads it, ! before it for a locked account; # comments),
// read at each login so that edits take effect at once
type File struct {
	path   string
	scheme string
}

// NewFile: the [file] backend, its users file checked to exist
func NewFile(s *config.Section) (Authenticator, error) {
	a := &File{
		path:   s.Get("path", "/etc/asterisk/modemmenu.users"),
		scheme: strings.ToLower(s.Get("hash", "auto")),
	}
	if _, err := os.Stat(a.path); err != nil {
		return nil, fmt.Errorf("[file] path: %v", err)
	}
	return a, nil
}

// Name: the backend's Name, for {auth} and the log
func (a *File) Name() string { return "file" }

// Authenticate: the user's line in the users file and its hash
// checked; a locked entry (!) is refused
func (a *File) Authenticate(_ context.Context, user, password string) error {
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
			Burn(password)
			return fmt.Errorf("%w: %s is locked", ErrBadPassword, user)
		}
		// user:hash[:anything else, e.g. a full name]
		if a.scheme != "plain" && !strings.HasPrefix(stored, "{") {
			stored, _, _ = strings.Cut(stored, ":")
		}
		ok, err := CheckPassword(stored, password, a.scheme)
		if err != nil {
			return fmt.Errorf("%s: user %s: %v", a.path, user, err)
		}
		if !ok {
			return ErrBadPassword
		}
		return nil
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return ErrNoUser
}
