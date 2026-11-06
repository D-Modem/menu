// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/D-Modem/menu/internal/config"
	"golang.org/x/crypto/bcrypt"
)

// TestCheckPassword: each stored hash scheme accepts the right password
// and refuses others
func TestCheckPassword(t *testing.T) {
	bc, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	s256 := sha256.Sum256([]byte("pw"))
	s512 := sha512.Sum512([]byte("pw"))
	salted := sha256.Sum256([]byte("pwSALT"))
	ssha := base64.StdEncoding.EncodeToString(append(salted[:], "SALT"...))
	for _, c := range []struct {
		stored, scheme string
		ok             bool
	}{
		{string(bc), "auto", true},
		{string(bc), "bcrypt", true},
		{"{SHA256}" + hex.EncodeToString(s256[:]), "auto", true},
		{"{SHA256}" + base64.StdEncoding.EncodeToString(s256[:]), "auto", true},
		{"{SHA512}" + hex.EncodeToString(s512[:]), "auto", true},
		{"{SSHA256}" + ssha, "auto", true},
		{"{PLAIN}pw", "auto", true},
		{"{PLAIN}px", "auto", false},
		{"pw", "plain", true},
		{hex.EncodeToString(s256[:]), "sha256", true},
	} {
		for _, pw := range []string{"pw", "wrong"} {
			ok, err := CheckPassword(c.stored, pw, c.scheme)
			if err != nil {
				t.Errorf("%s (%s): %v", c.stored, c.scheme, err)
			}
			if want := c.ok && pw == "pw"; ok != want {
				t.Errorf("%s (%s) %s: %v, want %v", c.stored, c.scheme, pw, ok, want)
			}
		}
	}
	if _, err := CheckPassword("pw", "pw", "auto"); err == nil {
		t.Error("a bare password with hash = auto: no error")
	}
}

// TestBackends: the file and SQL backends, in order, accept and refuse
// as they should
func TestBackends(t *testing.T) {
	dir := t.TempDir()
	users := filepath.Join(dir, "users")
	bc, _ := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	os.WriteFile(users, []byte("# users\nalice:"+string(bc)+":Alice\nbob:{PLAIN}a:b\n"), 0o600)
	fa, err := NewFile(&config.Section{Name: "file", Vals: map[string]string{"path": users}})
	if err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(dir, "users.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE users (username TEXT, password TEXT);
		INSERT INTO users VALUES ('carol', '{PLAIN}pw'), ('alice', '{PLAIN}other');`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	sa, err := NewSQL(&config.Section{Name: "sql", Vals: map[string]string{
		"driver": "sqlite", "dsn": dbPath,
		"query": "SELECT password FROM users WHERE username = ?"}})
	if err != nil {
		t.Fatal(err)
	}
	sn, err := NewSQL(&config.Section{Name: "sql", Vals: map[string]string{
		"driver": "sqlite", "dsn": dbPath, "args": "user password", "hash": "none",
		"query": "SELECT 1 FROM users WHERE username = ? AND password = '{PLAIN}' || ?"}})
	if err != nil {
		t.Fatal(err)
	}

	backends := []Authenticator{fa, sa}
	nolog := func(string, ...any) {}
	for _, c := range []struct {
		user, pw, auth string
		err            error
	}{
		{"alice", "secret", "file", nil},
		{"alice", "other", "", ErrBadPassword}, // the file knows alice: not asked further
		{"bob", "a:b", "file", nil},
		{"carol", "pw", "sql", nil},
		{"carol", "px", "", ErrBadPassword},
		{"dave", "pw", "", ErrNoUser},
	} {
		auth, err := Authenticate(context.Background(), backends, c.user, c.pw, nolog)
		if auth != c.auth || !errors.Is(err, c.err) && !(err == nil && c.err == nil) {
			t.Errorf("%s/%s: %q %v, want %q %v", c.user, c.pw, auth, err, c.auth, c.err)
		}
	}
	if err := sn.Authenticate(context.Background(), "carol", "pw"); err != nil {
		t.Errorf("hash = none: %v", err)
	}
	if err := sn.Authenticate(context.Background(), "carol", "px"); !errors.Is(err, ErrNoUser) {
		t.Errorf("hash = none, wrong password: %v", err)
	}
}
