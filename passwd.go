// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/sys/unix"
)

// errRefused: the backend did not take the new password (its policy)
var errRefused = errors.New("new password refused")

// passwordChanger: a backend that can change a user's password; old has
// been checked already.  t is the caller's line, for a backend (PAM) that
// has something to tell them.
type passwordChanger interface {
	changePassword(ctx context.Context, t *term, user, old, new string) error
}

// changePasswordItem: mode = passwd, the caller's password changed in
// the backend that holds their account
func changePasswordItem(t *term, s *settings, it *menuItem, sess *session, deadline time.Time) error {
	if sess.Auth == "guest" {
		return t.write("\r\nA guest has no password to change.\r\n")
	}
	end := time.Now().Add(s.loginTimeout)
	if !deadline.IsZero() && deadline.Before(end) {
		end = deadline
	}
	timeUp := func(err error) error {
		if errors.Is(err, errTimeout) && !deadline.IsZero() && !time.Now().Before(deadline) {
			return errTimeLimit
		}
		if errors.Is(err, errTimeout) || errors.Is(err, io.EOF) {
			t.write("\r\nYour password is as it was.\r\n")
			return nil
		}
		return err
	}

	if err := t.write("\r\nCurrent password: "); err != nil {
		return err
	}
	old, err := t.readLine(end, s.maxInput*2, false, s.mask)
	if err != nil {
		return timeUp(err)
	}
	// which backend holds the account: the one that takes the password
	// (a login by caller ID or an upstream menu did not say)
	ctx, cancel := context.WithDeadline(context.Background(), end.Add(10*time.Second))
	defer cancel()
	name, err := authenticate(ctx, s.backends, sess.User, old, logf)
	if err != nil {
		logf("passwd user=%s: current password wrong (%v)", sess.User, err)
		time.Sleep(s.failDelay)
		return t.write("That is not your password.\r\n")
	}
	var changer passwordChanger
	for _, b := range s.backends {
		if b.name() == name {
			changer, _ = b.(passwordChanger)
			if sa, ok := b.(*sqlAuth); ok && sa.updateQuery == "" {
				changer = nil
			}
		}
	}
	if changer == nil {
		logf("passwd user=%s: %s cannot change passwords", sess.User, name)
		return t.write("Your password cannot be changed here.  Ask the sysop.\r\n")
	}

	var pw string
	for try := 0; ; try++ {
		if try == 3 {
			return t.write("Your password is as it was.\r\n")
		}
		if err := t.write("New password: "); err != nil {
			return err
		}
		p, err := t.readLine(end, s.maxInput*2, false, s.mask)
		if err != nil {
			return timeUp(err)
		}
		switch {
		case p == "":
			return t.write("Your password is as it was.\r\n")
		case len(p) < it.minPassword:
			t.write(fmt.Sprintf("At least %d characters, please.\r\n", it.minPassword))
			continue
		case p == old:
			t.write("That is your password now; another one, please.\r\n")
			continue
		}
		if err := t.write("Again: "); err != nil {
			return err
		}
		again, err := t.readLine(end, s.maxInput*2, false, s.mask)
		if err != nil {
			return timeUp(err)
		}
		if again != p {
			t.write("They differ.\r\n")
			continue
		}
		pw = p
		break
	}

	if err := changer.changePassword(ctx, t, sess.User, old, pw); err != nil {
		logf("passwd user=%s auth=%s: %v", sess.User, name, err)
		if errors.Is(err, errRefused) {
			return t.write("The new password was not accepted.  Your password is as it was.\r\n")
		}
		return t.write("Sorry, your password cannot be changed now.\r\n")
	}
	sess.Password = pw
	logf("passwd user=%s auth=%s: changed", sess.User, name)
	return t.write("Your password is changed.\r\n")
}

// changePassword in the users file: the user's line rewritten in place
// (other lines as they are) under the lock signups take, the password as
// bcrypt (as text with hash = plain), a note after it kept
func (a *fileAuth) changePassword(_ context.Context, _ *term, user, old, new string) error {
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
		return errNoUser
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

// changePassword in the database: update_query with update_args, the
// password as update_hash says
func (a *sqlAuth) changePassword(ctx context.Context, _ *term, user, old, new string) error {
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

// changePassword by PAM: the service's password stack
func (a *systemAuth) changePassword(_ context.Context, t *term, user, old, new string) error {
	return pamChangePassword(a.service, user, old, new, t)
}
