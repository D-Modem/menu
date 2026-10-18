// Copyright (C) 2026 Neo-Desktop
// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFileChangePassword: the users file's line of the user changed,
// the others and the notes after the hash kept
func TestFileChangePassword(t *testing.T) {
	dir := t.TempDir()
	users := filepath.Join(dir, "users")
	os.WriteFile(users, []byte("# users\nalice:{PLAIN}old-pw\nbob:{PLAIN}bob-pw\ncarol:$2a$04$abcdefghijklmnopqrstuuJ8Jq7QF7mHNXgNnrBX3/7uJpH9fC2T6:signup 2026-10-12 5551234\n"), 0o600)
	fa, err := newFileAuth(&section{name: "file", vals: map[string]string{"path": users}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := fa.(*fileAuth).changePassword(ctx, nil, "alice", "old-pw", "new-pw-1"); err != nil {
		t.Fatal(err)
	}
	if err := fa.authenticate(ctx, "alice", "new-pw-1"); err != nil {
		t.Errorf("the new password: %v", err)
	}
	if err := fa.authenticate(ctx, "alice", "old-pw"); !errors.Is(err, errBadPassword) {
		t.Errorf("the old password: %v", err)
	}
	if err := fa.authenticate(ctx, "bob", "bob-pw"); err != nil {
		t.Errorf("bob, untouched: %v", err)
	}
	// a note after the hash stays
	if err := fa.(*fileAuth).changePassword(ctx, nil, "carol", "", "carol-new"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(users)
	lines := strings.Split(string(b), "\n")
	if len(lines) != 5 || lines[0] != "# users" || !strings.HasPrefix(lines[1], "alice:$2a$") ||
		lines[2] != "bob:{PLAIN}bob-pw" || !strings.HasSuffix(lines[3], ":signup 2026-10-12 5551234") || lines[4] != "" {
		t.Errorf("users file:\n%s", b)
	}
	if err := fa.(*fileAuth).changePassword(ctx, nil, "dave", "", "x"); !errors.Is(err, errNoUser) {
		t.Errorf("dave: %v", err)
	}

	// hash = plain: the password as text
	os.WriteFile(users, []byte("erin:old:x\n"), 0o600)
	fp, _ := newFileAuth(&section{name: "file", vals: map[string]string{"path": users, "hash": "plain"}})
	fp.(*fileAuth).changePassword(ctx, nil, "erin", "old:x", "new")
	if b, _ := os.ReadFile(users); string(b) != "erin:new\n" {
		t.Errorf("plain: %q", b)
	}
}

// TestSQLChangePassword: update_query changes the user's row only
func TestSQLChangePassword(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "users.db")
	db, _ := sql.Open("sqlite", dbPath)
	if _, err := db.Exec(`CREATE TABLE users (username TEXT, password TEXT);
		INSERT INTO users VALUES ('carol', '{PLAIN}pw'), ('dave', '{PLAIN}pw');`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	vals := map[string]string{"driver": "sqlite", "dsn": dbPath,
		"query": "SELECT password FROM users WHERE username = ?"}
	a, err := newSQLAuth(&section{name: "sql", vals: vals})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	vals["update_query"] = "UPDATE users SET password = ? WHERE username = ?"
	a, _ = newSQLAuth(&section{name: "sql", vals: vals})
	if err := a.(*sqlAuth).changePassword(ctx, nil, "carol", "pw", "carol-new"); err != nil {
		t.Fatal(err)
	}
	if err := a.authenticate(ctx, "carol", "carol-new"); err != nil {
		t.Errorf("the new password: %v", err)
	}
	if err := a.authenticate(ctx, "dave", "pw"); err != nil {
		t.Errorf("dave, untouched: %v", err)
	}
	if err := a.(*sqlAuth).changePassword(ctx, nil, "nobody", "pw", "x"); err == nil {
		t.Error("no row: no error")
	}
	vals["update_args"] = "user nonsense"
	if _, err := newSQLAuth(&section{name: "sql", vals: vals}); err == nil {
		t.Error("update_args nonsense: no error")
	}
}

// TestPasswdLine: on a line, a mode = passwd item changes the password
func TestPasswdLine(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "modemmenu")
	if out, err := runGo("build", "-o", bin, "."); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	users := filepath.Join(dir, "users")
	os.WriteFile(users, []byte("alice:{PLAIN}old-password\n"), 0o600)
	conf := filepath.Join(dir, "c.conf")
	os.WriteFile(conf, []byte(fmt.Sprintf(`
[general]
log = %s
auth = file
fail_delay = 0
[file]
path = %s
[guest]
enabled = yes
[item passwd]
key = PW
label = Change your password
mode = passwd
guest = yes
[item info]
command = echo "PW=$MODEM_PASSWORD"
after = menu
`, filepath.Join(dir, "log"), users)), 0o600)

	c, cmd := callAs(t, bin, conf)
	c.expect("login: ")
	c.send("alice\r")
	c.expect("Password: ")
	c.send("old-password\r")
	c.expect("Selection: ")
	c.send("pw\r")
	c.expect("Current password: ")
	c.send("wrong\r")
	c.expect("That is not your password.")
	c.expect("Selection: ")
	c.send("passwd\r")
	c.expect("Current password: ")
	c.send("old-password\r")
	c.expect("New password: ")
	c.send("short\r")
	c.expect("At least 8")
	c.expect("New password: ")
	c.send("old-password\r")
	c.expect("another one")
	c.expect("New password: ")
	c.send("new-password\r")
	c.expect("Again: ")
	c.send("new-passwork\r")
	c.expect("They differ.")
	// three tries
	c.expect("Your password is as it was.")
	c.expect("Selection: ")
	c.send("pw\r")
	c.expect("Current password: ")
	c.send("old-password\r")
	c.expect("New password: ")
	c.send("new-password\r")
	c.expect("Again: ")
	c.send("new-password\r")
	c.expect("Your password is changed.")
	c.expect("Selection: ")
	// the selections after it get the new one
	c.send("info\r")
	c.expect("PW=new-password")
	c.send("g\r")
	waitExit(t, cmd)

	c, cmd = callAs(t, bin, conf)
	c.expect("login: ")
	c.send("alice\r")
	c.expect("Password: ")
	c.send("old-password\r")
	c.expect("Login incorrect")
	c.expect("login: ")
	c.send("alice\r")
	c.expect("Password: ")
	c.send("new-password\r")
	c.expect("Selection: ")
	c.send("g\r")
	waitExit(t, cmd)

	// a guest has none
	c, cmd = callAs(t, bin, conf)
	c.expect("login: ")
	c.send("guest\r")
	c.expect("Selection: ")
	c.send("pw\r")
	c.expect("A guest has no password")
	c.send("g\r")
	waitExit(t, cmd)

	log, _ := os.ReadFile(filepath.Join(dir, "log"))
	for _, w := range []string{"passwd user=alice: current password wrong", "passwd user=alice auth=file: changed"} {
		if !strings.Contains(string(log), w) {
			t.Errorf("no %q in the log:\n%s", w, log)
		}
	}
	if strings.Contains(string(log), "new-password") {
		t.Errorf("a password in the log:\n%s", log)
	}
}

// MODEMMENU_TEST_LDAP (see ldap_test.go), the server letting users write
// their own userPassword: lduser's password changed and back
func TestLDAPChangePassword(t *testing.T) {
	url := os.Getenv("MODEMMENU_TEST_LDAP")
	if url == "" {
		t.Skip("MODEMMENU_TEST_LDAP is not set")
	}
	a, err := newLDAPAuth(&section{name: "ldap", vals: map[string]string{
		"url": url, "search_base": "ou=people,dc=example,dc=com"}})
	if err != nil {
		t.Fatal(err)
	}
	la := a.(*ldapAuth)
	ctx := context.Background()
	if err := la.changePassword(ctx, nil, "lduser", "wrong", "x"); !errors.Is(err, errBadPassword) {
		t.Errorf("a wrong current password: %v", err)
	}
	if err := la.changePassword(ctx, nil, "lduser", "ldpass", "ldpass-2"); err != nil {
		t.Fatal(err)
	}
	if err := a.authenticate(ctx, "lduser", "ldpass-2"); err != nil {
		t.Errorf("the new password: %v", err)
	}
	if err := a.authenticate(ctx, "lduser", "ldpass"); err == nil {
		t.Error("the old password still works")
	}
	if err := la.changePassword(ctx, nil, "lduser", "ldpass-2", "ldpass"); err != nil {
		t.Errorf("back: %v", err)
	}
}
