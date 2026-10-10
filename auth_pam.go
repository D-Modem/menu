//go:build pam

package main

import (
	"errors"
	"fmt"

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
