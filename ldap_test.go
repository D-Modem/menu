// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
		a, err := newLDAPAuth(&section{name: "ldap", vals: vals})
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if err := a.authenticate(ctx, "lduser", "ldpass"); err != nil {
			t.Errorf("%v: right password: %v", vals, err)
		}
		if err := a.authenticate(ctx, "lduser", "wrong"); err == nil {
			t.Errorf("%v: wrong password accepted", vals)
		}
		if err := a.authenticate(ctx, "lduser", ""); err == nil {
			t.Errorf("%v: empty password accepted", vals)
		}
		if err := a.authenticate(ctx, "nobody*)(uid=*", "ldpass"); !errors.Is(err, errNoUser) {
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
		a, err := newLDAPAuth(&section{name: "ldap", vals: vals})
		if err != nil {
			t.Fatal(err)
		}
		la := a.(*ldapAuth)
		for group, want := range map[string]bool{"sysops": true, "dialup": true, "other": false, "nothere": false} {
			if in, err := la.memberOf("lduser", "ldpass", group); err != nil || in != want {
				t.Errorf("%v: lduser in %s: %v %v, want %v", vals, group, in, err, want)
			}
		}
	}
	a, _ := newLDAPAuth(&section{name: "ldap", vals: map[string]string{"url": url, "search_base": "ou=people,dc=example,dc=com"}})
	if _, err := a.(*ldapAuth).memberOf("lduser", "ldpass", "sysops"); !errors.Is(err, errNoUser) {
		t.Errorf("without group_base: %v", err)
	}

	// an item for sysops, through settings, a login by ldap
	dir := t.TempDir()
	p := filepath.Join(dir, "c.conf")
	os.WriteFile(p, []byte("[general]\nauth = ldap\n[ldap]\nurl = "+url+
		"\nsearch_base = ou=people,dc=example,dc=com\ngroup_base = ou=groups,dc=example,dc=com\n"+
		"[item ops]\ngroups = sysops\ncommand = true\n[item away]\ngroups = other\ncommand = true\n"), 0o600)
	s, err := loadSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	sess := &session{User: "lduser", Password: "ldpass", Auth: "ldap"}
	var names []string
	for _, it := range s.visible(sess) {
		names = append(names, it.name)
	}
	if strings.Join(names, " ") != "ops" {
		t.Errorf("lduser sees %v", names)
	}
}
