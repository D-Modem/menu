// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/sys/unix"
)

// ErrRefused: the backend did not take the new password (its policy)
var ErrRefused = errors.New("new password refused")

// PasswordChanger: a backend that can change a user's password; old has
// been checked already.  t is the caller's line, for a backend (PAM) that
// has something to tell them.
type PasswordChanger interface {
	ChangePassword(ctx context.Context, t Messenger, user, old, new string) error
	// CanChangePassword: whether this backend is set up to (sql: an
	// update_query)
	CanChangePassword() bool
}

// Messenger: the caller's line, for what a backend has to tell them
type Messenger interface {
	Write(s string) error
}

func (a *File) CanChangePassword() bool   { return true }
func (a *SQL) CanChangePassword() bool    { return a.updateQuery != "" }
func (a *System) CanChangePassword() bool { return true }

// ChangePassword in the users file: the user's line rewritten in place
// (other lines as they are) under the lock signups take, the password as
// bcrypt (as text with hash = plain), a note after it kept
func (a *File) ChangePassword(_ context.Context, _ Messenger, user, old, new string) error {
	f, err := os.OpenFile(a.path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		return err
	}
	b := make([]byte, st.Size())
	if _, err := f.ReadAt(b, 0); err != nil && err != io.EOF {
		return err
	}
	stored := ""
	if a.scheme == "plain" {
		stored = new
	} else {
		h, err := bcrypt.GenerateFromPassword([]byte(new), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		stored = string(h)
	}
	lines := strings.Split(string(b), "\n")
	found := false
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || t[0] == '#' {
			continue
		}
		name, rest, ok := strings.Cut(t, ":")
		if !ok || name != user {
			continue
		}
		// what follows the hash: user:hash:note (not after {SCHEME}text
		// or with hash = plain, where the rest is the password)
		note := ""
		if a.scheme != "plain" && !strings.HasPrefix(rest, "{") {
			if _, n, ok := strings.Cut(rest, ":"); ok {
				note = ":" + n
			}
		}
		lines[i] = name + ":" + stored + note
		found = true
		break
	}
	if !found {
		return ErrNoUser
	}
	out := []byte(strings.Join(lines, "\n"))
	if _, err := f.WriteAt(out, 0); err != nil {
		return err
	}
	if err := f.Truncate(int64(len(out))); err != nil {
		return err
	}
	return f.Sync()
}

// ChangePassword in the database: update_query with update_args, the
// password as update_hash says
func (a *SQL) ChangePassword(ctx context.Context, _ Messenger, user, old, new string) error {
	stored := new
	if a.updateHash == "bcrypt" {
		h, err := bcrypt.GenerateFromPassword([]byte(new), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		stored = string(h)
	}
	args := make([]any, len(a.updateArgs))
	for i, arg := range a.updateArgs {
		switch arg {
		case "user":
			args[i] = user
		case "password":
			args[i] = stored
		case "old_password":
			args[i] = old
		}
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	res, err := a.db.ExecContext(ctx, a.updateQuery, args...)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n != 1 {
		return fmt.Errorf("update_query changed %d rows, not 1", n)
	}
	return nil
}

// ChangePassword by PAM: the service's password stack
func (a *System) ChangePassword(_ context.Context, t Messenger, user, old, new string) error {
	return pamChangePassword(a.service, user, old, new, t)
}
