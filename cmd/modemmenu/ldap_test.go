// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// MODEMMENU_TEST_LDAP, groups as in internal/auth's TestLDAPGroups: an
// item for sysops, through settings, a login by ldap
func TestLDAPItemGroups(t *testing.T) {
	url := os.Getenv("MODEMMENU_TEST_LDAP")
	if url == "" {
		t.Skip("MODEMMENU_TEST_LDAP is not set")
	}
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
