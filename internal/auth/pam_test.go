// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

//go:build pam

package auth

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/D-Modem/menu/internal/config"
)

// MODEMMENU_TEST_PAM=user:password of an account the PAM service
// modemmenu (/etc/pam.d/modemmenu) accepts; run as root for pam_unix
func TestPAM(t *testing.T) {
	cred := os.Getenv("MODEMMENU_TEST_PAM")
	if cred == "" {
		t.Skip("MODEMMENU_TEST_PAM is not set")
	}
	user, pw, _ := strings.Cut(cred, ":")
	a, err := NewSystem(&config.Section{Name: "system", Vals: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := a.Authenticate(ctx, user, pw); err != nil {
		t.Errorf("right password: %v", err)
	}
	if err := a.Authenticate(ctx, user, pw+"x"); !errors.Is(err, ErrBadPassword) {
		t.Errorf("wrong password: %v", err)
	}
	if err := a.Authenticate(ctx, "no-such-user-here", pw); !errors.Is(err, ErrNoUser) {
		t.Errorf("unknown user: %v", err)
	}
	if err := a.Authenticate(ctx, "root", pw); !errors.Is(err, ErrNoUser) {
		t.Errorf("root: %v", err)
	}
}

// MODEMMENU_TEST_PAM, a password stack in /etc/pam.d/modemmenu: the
// account's password changed and back
func TestPAMChangePassword(t *testing.T) {
	cred := os.Getenv("MODEMMENU_TEST_PAM")
	if cred == "" {
		t.Skip("MODEMMENU_TEST_PAM is not set")
	}
	user, pw, _ := strings.Cut(cred, ":")
	a, err := NewSystem(&config.Section{Name: "system", Vals: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	sa := a.(*System)
	ctx := context.Background()
	if err := sa.ChangePassword(ctx, nil, user, pw, pw+"-2"); err != nil {
		t.Fatal(err)
	}
	if err := a.Authenticate(ctx, user, pw+"-2"); err != nil {
		t.Errorf("the new password: %v", err)
	}
	if err := a.Authenticate(ctx, user, pw); err == nil {
		t.Error("the old password still works")
	}
	if err := sa.ChangePassword(ctx, nil, user, pw+"-2", pw); err != nil {
		t.Errorf("back: %v", err)
	}
}
