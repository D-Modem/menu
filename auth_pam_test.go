//go:build pam

package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// MODEMMENU_TEST_PAM=user:password of an account the PAM service
// modemmenu (/etc/pam.d/modemmenu) accepts; run as root for pam_unix
func TestPAM(t *testing.T) {
	cred := os.Getenv("MODEMMENU_TEST_PAM")
	if cred == "" {
		t.Skip("MODEMMENU_TEST_PAM is not set")
	}
	user, pw, _ := strings.Cut(cred, ":")
	a, err := newSystemAuth(&section{name: "system", vals: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := a.authenticate(ctx, user, pw); err != nil {
		t.Errorf("right password: %v", err)
	}
	if err := a.authenticate(ctx, user, pw+"x"); !errors.Is(err, errBadPassword) {
		t.Errorf("wrong password: %v", err)
	}
	if err := a.authenticate(ctx, "no-such-user-here", pw); !errors.Is(err, errNoUser) {
		t.Errorf("unknown user: %v", err)
	}
	if err := a.authenticate(ctx, "root", pw); !errors.Is(err, errNoUser) {
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
	a, err := newSystemAuth(&section{name: "system", vals: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	sa := a.(*systemAuth)
	ctx := context.Background()
	if err := sa.changePassword(ctx, nil, user, pw, pw+"-2"); err != nil {
		t.Fatal(err)
	}
	if err := a.authenticate(ctx, user, pw+"-2"); err != nil {
		t.Errorf("the new password: %v", err)
	}
	if err := a.authenticate(ctx, user, pw); err == nil {
		t.Error("the old password still works")
	}
	if err := sa.changePassword(ctx, nil, user, pw+"-2", pw); err != nil {
		t.Errorf("back: %v", err)
	}
}
