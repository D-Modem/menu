// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/D-Modem/menu/internal/config"
	"github.com/go-ldap/ldap/v3"
)

// MODEMMENU_TEST_LDAP=ldap://host:port of a server with
// uid=lduser,ou=people,dc=example,dc=com, password ldpass
func TestLDAP(t *testing.T) {
	url := os.Getenv("MODEMMENU_TEST_LDAP")
	if url == "" {
		t.Skip("MODEMMENU_TEST_LDAP is not set")
	}
	for _, vals := range []map[string]string{
		{"url": url, "search_base": "ou=people,dc=example,dc=com"},
		{"url": url, "bind_dn": "uid={user},ou=people,dc=example,dc=com"},
	} {
		a, err := NewLDAP(&config.Section{Name: "ldap", Vals: vals})
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if err := a.Authenticate(ctx, "lduser", "ldpass"); err != nil {
			t.Errorf("%v: right password: %v", vals, err)
		}
		if err := a.Authenticate(ctx, "lduser", "wrong"); err == nil {
			t.Errorf("%v: wrong password accepted", vals)
		}
		if err := a.Authenticate(ctx, "lduser", ""); err == nil {
			t.Errorf("%v: empty password accepted", vals)
		}
		if err := a.Authenticate(ctx, "nobody*)(uid=*", "ldpass"); !errors.Is(err, ErrNoUser) {
			t.Errorf("%v: unknown user: %v", vals, err)
		}
	}
}

// TestADPassword: unicodePwd's encoding, and AD's change request
func TestADPassword(t *testing.T) {
	// "pw" in quotes, UTF-16LE
	if got := adPassword("pw"); got != "\x22\x00p\x00w\x00\x22\x00" {
		t.Errorf("% x", got)
	}
	if got := adPassword("é"); got != "\x22\x00\xe9\x00\x22\x00" {
		t.Errorf("% x", got)
	}
	m := adPasswordChange("cn=a,dc=x", "old", "new")
	if len(m.Changes) != 2 || m.Changes[0].Operation != ldap.DeleteAttribute ||
		m.Changes[1].Operation != ldap.AddAttribute ||
		m.Changes[1].Modification.Vals[0] != adPassword("new") {
		t.Errorf("modify %+v", m.Changes)
	}
}

// MODEMMENU_TEST_LDAP, with ou=groups,dc=example,dc=com holding
// cn=sysops (groupOfNames, member lduser) and cn=dialup (posixGroup,
// memberUid lduser) and cn=other (without lduser)
func TestLDAPGroups(t *testing.T) {
	url := os.Getenv("MODEMMENU_TEST_LDAP")
	if url == "" {
		t.Skip("MODEMMENU_TEST_LDAP is not set")
	}
	for _, vals := range []map[string]string{
		{"url": url, "search_base": "ou=people,dc=example,dc=com", "group_base": "ou=groups,dc=example,dc=com"},
		{"url": url, "bind_dn": "uid={user},ou=people,dc=example,dc=com", "group_base": "ou=groups,dc=example,dc=com"},
	} {
		a, err := NewLDAP(&config.Section{Name: "ldap", Vals: vals})
		if err != nil {
			t.Fatal(err)
		}
		la := a.(*LDAP)
		for group, want := range map[string]bool{"sysops": true, "dialup": true, "other": false, "nothere": false} {
			if in, err := la.MemberOf("lduser", "ldpass", group); err != nil || in != want {
				t.Errorf("%v: lduser in %s: %v %v, want %v", vals, group, in, err, want)
			}
		}
	}
	a, _ := NewLDAP(&config.Section{Name: "ldap", Vals: map[string]string{"url": url, "search_base": "ou=people,dc=example,dc=com"}})
	if _, err := a.(*LDAP).MemberOf("lduser", "ldpass", "sysops"); !errors.Is(err, ErrNoUser) {
		t.Errorf("without group_base: %v", err)
	}
}
