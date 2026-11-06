// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/D-Modem/menu/internal/auth"
	"github.com/D-Modem/menu/internal/term"
)

// changePasswordItem: mode = passwd, the caller's password changed in
// the backend that holds their account
func changePasswordItem(t *term.Term, s *settings, it *menuItem, sess *session, deadline time.Time) error {
	if sess.Auth == "guest" {
		return t.Write("\r\nA guest has no password to change.\r\n")
	}
	end := time.Now().Add(s.loginTimeout)
	if !deadline.IsZero() && deadline.Before(end) {
		end = deadline
	}
	timeUp := func(err error) error {
		if errors.Is(err, term.ErrTimeout) && !deadline.IsZero() && !time.Now().Before(deadline) {
			return errTimeLimit
		}
		if errors.Is(err, term.ErrTimeout) || errors.Is(err, io.EOF) {
			t.Write("\r\nYour password is as it was.\r\n")
			return nil
		}
		return err
	}

	if err := t.Write("\r\nCurrent password: "); err != nil {
		return err
	}
	old, err := t.ReadLine(end, s.maxInput*2, false, s.mask)
	if err != nil {
		return timeUp(err)
	}
	// which backend holds the account: the one that takes the password
	// (a login by caller ID or an upstream menu did not say)
	ctx, cancel := context.WithDeadline(context.Background(), end.Add(10*time.Second))
	defer cancel()
	name, err := auth.Authenticate(ctx, s.backends, sess.User, old, logf)
	if err != nil {
		logf("passwd user=%s: current password wrong (%v)", sess.User, err)
		time.Sleep(s.failDelay)
		return t.Write("That is not your password.\r\n")
	}
	var changer auth.PasswordChanger
	for _, b := range s.backends {
		if b.Name() == name {
			if c, ok := b.(auth.PasswordChanger); ok && c.CanChangePassword() {
				changer = c
			}
		}
	}
	if changer == nil {
		logf("passwd user=%s: %s cannot change passwords", sess.User, name)
		return t.Write("Your password cannot be changed here.  Ask the sysop.\r\n")
	}

	var pw string
	for try := 0; ; try++ {
		if try == 3 {
			return t.Write("Your password is as it was.\r\n")
		}
		if err := t.Write("New password: "); err != nil {
			return err
		}
		p, err := t.ReadLine(end, s.maxInput*2, false, s.mask)
		if err != nil {
			return timeUp(err)
		}
		switch {
		case p == "":
			return t.Write("Your password is as it was.\r\n")
		case len(p) < it.minPassword:
			t.Write(fmt.Sprintf("At least %d characters, please.\r\n", it.minPassword))
			continue
		case p == old:
			t.Write("That is your password now; another one, please.\r\n")
			continue
		}
		if err := t.Write("Again: "); err != nil {
			return err
		}
		again, err := t.ReadLine(end, s.maxInput*2, false, s.mask)
		if err != nil {
			return timeUp(err)
		}
		if again != p {
			t.Write("They differ.\r\n")
			continue
		}
		pw = p
		break
	}

	if err := changer.ChangePassword(ctx, t, sess.User, old, pw); err != nil {
		logf("passwd user=%s auth=%s: %v", sess.User, name, err)
		if errors.Is(err, auth.ErrRefused) {
			return t.Write("The new password was not accepted.  Your password is as it was.\r\n")
		}
		return t.Write("Sorry, your password cannot be changed now.\r\n")
	}
	sess.Password = pw
	logf("passwd user=%s auth=%s: changed", sess.User, name)
	return t.Write("Your password is changed.\r\n")
}
