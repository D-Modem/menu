//go:build pam

package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/msteinert/pam/v2"
)

const havePAM = true

// pamAuthenticate: the PAM service's auth and account stacks for the
// user, the password the answer to its hidden prompts
func pamAuthenticate(service, user, password string) error {
	t, err := pam.StartFunc(service, user, func(s pam.Style, msg string) (string, error) {
		switch s {
		case pam.PromptEchoOff:
			return password, nil
		case pam.PromptEchoOn:
			return user, nil
		case pam.ErrorMsg, pam.TextInfo:
			return "", nil
		}
		return "", fmt.Errorf("unsupported PAM message style %d", s)
	})
	if err != nil {
		return fmt.Errorf("pam %s: %v", service, err)
	}
	defer t.End()
	t.SetItem(pam.Tty, "modem")
	if err := t.Authenticate(pam.DisallowNullAuthtok); err != nil {
		if errors.Is(err, pam.ErrUserUnknown) {
			return errNoUser
		}
		if errors.Is(err, pam.ErrAuth) || errors.Is(err, pam.ErrPermDenied) ||
			errors.Is(err, pam.ErrMaxtries) || errors.Is(err, pam.ErrCredInsufficient) {
			return errBadPassword
		}
		return fmt.Errorf("pam %s: %v", service, err)
	}
	if err := t.AcctMgmt(pam.DisallowNullAuthtok); err != nil {
		// expired, locked, outside its hours...: refused, and why logged
		return fmt.Errorf("%w: pam account %s: %v", errBadPassword, user, err)
	}
	return nil
}

// pamChangePassword: the PAM service's password stack for the user.  Its
// hidden prompts get the current password (asked for "current" or "old")
// or the new one; what it has to say goes to the caller (a quality
// module's reasons).
func pamChangePassword(service, user, old, new string, line *term) error {
	t, err := pam.StartFunc(service, user, func(s pam.Style, msg string) (string, error) {
		switch s {
		case pam.PromptEchoOff:
			m := strings.ToLower(msg)
			if strings.Contains(m, "current") || strings.Contains(m, "old") {
				return old, nil
			}
			return new, nil
		case pam.PromptEchoOn:
			return user, nil
		case pam.ErrorMsg, pam.TextInfo:
			if line != nil && msg != "" {
				line.write(crlf(strings.TrimRight(msg, "\n")) + "\r\n")
			}
			return "", nil
		}
		return "", fmt.Errorf("unsupported PAM message style %d", s)
	})
	if err != nil {
		return fmt.Errorf("pam %s: %v", service, err)
	}
	defer t.End()
	t.SetItem(pam.Tty, "modem")
	if err := t.ChangeAuthTok(0); err != nil {
		if errors.Is(err, pam.ErrAuthtok) || errors.Is(err, pam.ErrPermDenied) ||
			errors.Is(err, pam.ErrAuthtokRecovery) || errors.Is(err, pam.ErrAuth) {
			return fmt.Errorf("%w: pam %s: %v", errRefused, service, err)
		}
		return fmt.Errorf("pam %s: %v", service, err)
	}
	return nil
}
