// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/D-Modem/menu/internal/config"
)

// TestFileChangePassword: the users file's line of the user changed,
// the others and the notes after the hash kept
func TestFileChangePassword(t *testing.T) {
	dir := t.TempDir()
	users := filepath.Join(dir, "users")
	os.WriteFile(users, []byte("# users\nalice:{PLAIN}old-pw\nbob:{PLAIN}bob-pw\ncarol:$2a$04$abcdefghijklmnopqrstuuJ8Jq7QF7mHNXgNnrBX3/7uJpH9fC2T6:signup 2026-10-12 5551234\n"), 0o600)
	fa, err := NewFile(&config.Section{Name: "file", Vals: map[string]string{"path": users}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := fa.(*File).ChangePassword(ctx, nil, "alice", "old-pw", "new-pw-1"); err != nil {
		t.Fatal(err)
	}
	if err := fa.Authenticate(ctx, "alice", "new-pw-1"); err != nil {
		t.Errorf("the new password: %v", err)
	}
	if err := fa.Authenticate(ctx, "alice", "old-pw"); !errors.Is(err, ErrBadPassword) {
		t.Errorf("the old password: %v", err)
	}
	if err := fa.Authenticate(ctx, "bob", "bob-pw"); err != nil {
		t.Errorf("bob, untouched: %v", err)
	}
	// a note after the hash stays
	if err := fa.(*File).ChangePassword(ctx, nil, "carol", "", "carol-new"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(users)
	lines := strings.Split(string(b), "\n")
	if len(lines) != 5 || lines[0] != "# users" || !strings.HasPrefix(lines[1], "alice:$2a$") ||
		lines[2] != "bob:{PLAIN}bob-pw" || !strings.HasSuffix(lines[3], ":signup 2026-10-12 5551234") || lines[4] != "" {
		t.Errorf("users file:\n%s", b)
	}
	if err := fa.(*File).ChangePassword(ctx, nil, "dave", "", "x"); !errors.Is(err, ErrNoUser) {
		t.Errorf("dave: %v", err)
	}

	// hash = plain: the password as text
	os.WriteFile(users, []byte("erin:old:x\n"), 0o600)
	fp, _ := NewFile(&config.Section{Name: "file", Vals: map[string]string{"path": users, "hash": "plain"}})
	fp.(*File).ChangePassword(ctx, nil, "erin", "old:x", "new")
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
	a, err := NewSQL(&config.Section{Name: "sql", Vals: vals})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	vals["update_query"] = "UPDATE users SET password = ? WHERE username = ?"
	a, _ = NewSQL(&config.Section{Name: "sql", Vals: vals})
	if err := a.(*SQL).ChangePassword(ctx, nil, "carol", "pw", "carol-new"); err != nil {
		t.Fatal(err)
	}
	if err := a.Authenticate(ctx, "carol", "carol-new"); err != nil {
		t.Errorf("the new password: %v", err)
	}
	if err := a.Authenticate(ctx, "dave", "pw"); err != nil {
		t.Errorf("dave, untouched: %v", err)
	}
	if err := a.(*SQL).ChangePassword(ctx, nil, "nobody", "pw", "x"); err == nil {
		t.Error("no row: no error")
	}
	vals["update_args"] = "user nonsense"
	if _, err := NewSQL(&config.Section{Name: "sql", Vals: vals}); err == nil {
		t.Error("update_args nonsense: no error")
	}
}

// MODEMMENU_TEST_LDAP (see ldap_test.go), the server letting users write
// their own userPassword: lduser's password changed and back
func TestLDAPChangePassword(t *testing.T) {
	url := os.Getenv("MODEMMENU_TEST_LDAP")
	if url == "" {
		t.Skip("MODEMMENU_TEST_LDAP is not set")
	}
	a, err := NewLDAP(&config.Section{Name: "ldap", Vals: map[string]string{
		"url": url, "search_base": "ou=people,dc=example,dc=com"}})
	if err != nil {
		t.Fatal(err)
	}
	la := a.(*LDAP)
	ctx := context.Background()
	if err := la.ChangePassword(ctx, nil, "lduser", "wrong", "x"); !errors.Is(err, ErrBadPassword) {
		t.Errorf("a wrong current password: %v", err)
	}
	if err := la.ChangePassword(ctx, nil, "lduser", "ldpass", "ldpass-2"); err != nil {
		t.Fatal(err)
	}
	if err := a.Authenticate(ctx, "lduser", "ldpass-2"); err != nil {
		t.Errorf("the new password: %v", err)
	}
	if err := a.Authenticate(ctx, "lduser", "ldpass"); err == nil {
		t.Error("the old password still works")
	}
	if err := la.ChangePassword(ctx, nil, "lduser", "ldpass-2", "ldpass"); err != nil {
		t.Errorf("back: %v", err)
	}
}
